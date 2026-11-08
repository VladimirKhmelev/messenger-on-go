package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSocketObservesReceiptBeforeAckAndKeepsJournal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "test-token" {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		for {
			var cmd map[string]string
			if err := ws.ReadJSON(&cmd); err != nil {
				return
			}
			if err := ws.WriteJSON(event{Type: "message_received", Chat: cmd["chat_id"], Message: &message{ID: "m", Sender: "alice", Text: cmd["text"]}}); err != nil {
				return
			}
			if err := ws.WriteJSON(event{Type: "message_sent", ID: "m"}); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	s, err := dialSocket(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	from := s.mark()
	id, err := s.ack(ctx, "chat", "opaque")
	if err != nil || id != "m" {
		t.Fatalf("ACK %s, %v", id, err)
	}
	if err := s.receipt(ctx, from, "chat", id, "opaque", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.receipt(ctx, from, "chat", id, "opaque", "bob"); err == nil {
		t.Fatal("wrong sender accepted")
	}
	m, d, o := inspectDelivery(s.snapshot(from), "chat", []string{id})
	if m+d+o != 0 {
		t.Fatalf("%d/%d/%d", m, d, o)
	}
	canceled, stop := context.WithCancel(t.Context())
	stop()
	if _, err := s.wait(canceled, from, func(e event) bool { return e.Type == "message_sent" }); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled wait returned a buffered event", err)
	}
}
func TestDeliveryInspectionCountsOrderDuplicatesAndMissing(t *testing.T) {
	events := []event{{Type: "notify_push", Chat: "chat", ID: "one"}, {Type: "message_received", Chat: "other", Message: &message{ID: "one"}},
		{Type: "message_received", Chat: "chat", Message: &message{ID: "two"}}, {Type: "message_received", Chat: "chat", Message: &message{ID: "one"}},
		{Type: "message_received", Chat: "chat", Message: &message{ID: "one"}}, {Type: "message_received", Chat: "chat", Message: &message{ID: "foreign"}}}
	m, d, o := inspectDelivery(events, "chat", []string{"one", "two", "three"})
	if m != 1 || d != 1 || o != 1 {
		t.Fatalf("missing=%d duplicates=%d order=%d", m, d, o)
	}
}
