package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fixturePeer struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (p *fixturePeer) write(e event) { p.mu.Lock(); defer p.mu.Unlock(); _ = p.ws.WriteJSON(e) }

// The first operation commits, but its response and notification are lost.
// The runner must resolve it via history rather than send it again.
func TestUncertainWriteIsNotRetriedAndBothClientsRecover(t *testing.T) {
	var mu sync.Mutex
	var stored []message
	peers := map[*fixturePeer]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			p := &fixturePeer{ws: ws}
			mu.Lock()
			peers[p] = true
			mu.Unlock()
			defer func() { mu.Lock(); delete(peers, p); mu.Unlock(); _ = ws.Close() }()
			for {
				var cmd struct {
					Type  string `json:"type"`
					Chat  string `json:"chat_id"`
					Text  string `json:"text"`
					Limit int    `json:"limit"`
				}
				if err := ws.ReadJSON(&cmd); err != nil {
					return
				}
				if cmd.Type == "get_history" {
					p.write(event{Type: "history", Chat: cmd.Chat})
					continue
				}
				mu.Lock()
				m := message{ID: fmt.Sprint(len(stored) + 1), Sender: r.URL.Query().Get("token"), Text: cmd.Text}
				stored = append(stored, m)
				all := make([]*fixturePeer, 0, len(peers))
				for peer := range peers {
					all = append(all, peer)
				}
				mu.Unlock()
				if strings.HasPrefix(m.Text, "uncertain:") {
					p.write(event{Type: "error", Error: "response lost"})
					continue
				}
				for _, peer := range all {
					peer.write(event{Type: "message_received", Chat: cmd.Chat, Message: &m})
				}
				p.write(event{Type: "message_sent", ID: m.ID})
			}
		}
		if r.URL.Path == "/v1/chats/chat/messages" {
			mu.Lock()
			defer mu.Unlock()
			messages := make([]map[string]any, 0, len(stored))
			for _, m := range stored {
				messages = append(messages, map[string]any{"messageId": m.ID, "senderUserId": m.Sender, "text": m.Text})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": messages})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()
	a := newAPI("")
	defer a.client.CloseIdleConnections()
	s := &suite{api: a, endpoints: endpoints{auth: srv.URL, chat: srv.URL, media: srv.URL}, rep: &report{Run: "test"}, chatID: "chat", observe: time.Millisecond}
	s.users[0] = user{ID: "alice"}
	s.users[1] = user{ID: "bob"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for i, token := range []string{"alice", "bob"} {
		sock, err := dialSocket(ctx, srv.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		s.sockets[i] = sock
		defer sock.close()
	}
	result := &caseResult{Details: map[string]any{}}
	actions := 0
	if err := s.uncertain(ctx, "probe", func() error { actions++; return nil }, result); err != nil {
		t.Fatal(err)
	}
	if err := s.audit(ctx, result); err != nil {
		t.Fatal(err)
	}
	if actions != 1 || result.Details["stored_copies"] != 1 {
		t.Fatalf("actions %d, details %v", actions, result.Details)
	}
	if got := result.Details["fault_gateway_2"].(deliveryResult); got.Missing != 1 {
		t.Fatalf("lost notification hidden: %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	uncertain := 0
	for _, m := range stored {
		if strings.HasPrefix(m.Text, "uncertain:") {
			uncertain++
		}
	}
	if uncertain != 1 {
		t.Fatalf("uncertain write was sent %d times", uncertain)
	}
}
func TestReadinessWaitsForBothGatewaysAndWorker(t *testing.T) {
	filters := []string{"msg.created", "msg.updated", "msg.deleted", "msg.read", "chat.deleted"}
	consumer := func(filter string) map[string]any {
		return map[string]any{"config": map[string]any{"filter_subject": filter}}
	}
	chat := map[string]any{"config": map[string]any{"filter_subjects": filters}}
	worker := map[string]any{"config": map[string]any{"durable_name": "notification-worker-chat-events"}}
	body := map[string]any{"account_details": []any{map[string]any{"stream_detail": []any{
		map[string]any{"name": "CHAT_EVENTS", "consumer_detail": []any{chat, chat, worker}},
		map[string]any{"name": "NOTIFY_EVENTS", "consumer_detail": []any{consumer("notify.push"), consumer("notify.push")}},
		map[string]any{"name": "USER_EVENTS", "consumer_detail": []any{consumer("user.profile_updated"), consumer("user.profile_updated")}},
	}}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("consumers") != "true" || r.URL.Query().Get("config") != "true" {
			t.Error("consumer detail and config must both be requested")
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()
	a := newAPI("")
	defer a.client.CloseIdleConnections()
	s := &suite{api: a, endpoints: endpoints{nats: srv.URL}}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.consumersReady(ctx); err != nil {
		t.Fatal(err)
	}
}
