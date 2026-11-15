package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRealPairReceivesAcrossConnections(t *testing.T) {
	type peerSocket struct {
		mu sync.Mutex
		ws *websocket.Conn
	}
	peers := map[string]*peerSocket{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			mu.Lock()
			ready := len(peers) == 2
			mu.Unlock()
			if ready {
				_, _ = fmt.Fprint(w, "ws_connections_active 1\n")
			} else {
				_, _ = fmt.Fprint(w, "ws_connections_active 0\n")
			}
			return
		}
		id := strings.TrimSuffix(r.URL.Query().Get("token"), "-secret")
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		p := &peerSocket{ws: ws}
		mu.Lock()
		peers[id] = p
		mu.Unlock()
		for {
			var m clientMessage
			if err := ws.ReadJSON(&m); err != nil {
				return
			}
			other := "a"
			if id == "a" {
				other = "b"
			}
			mu.Lock()
			dest := peers[other]
			mu.Unlock()
			if dest == nil {
				t.Error("receiver not ready")
				return
			}
			messageID := id + ":" + m.Text
			dest.mu.Lock()
			_ = dest.ws.WriteJSON(map[string]any{"type": "notify_push", "chat_id": "chat", "message_id": messageID})
			_ = dest.ws.WriteJSON(map[string]any{"type": "message_received", "chat_id": "chat", "message": map[string]string{"message_id": messageID, "text": m.Text}})
			dest.mu.Unlock()
			p.mu.Lock()
			_ = ws.WriteJSON(map[string]string{"type": "message_sent", "message_id": messageID})
			p.mu.Unlock()
		}
	}))
	defer srv.Close()
	f := fixture{Schema: 1, Users: []fixtureUser{{ID: "a", Token: "a-secret", ChatID: "chat", PeerID: "b", Gateway: 0}, {ID: "b", Token: "b-secret", ChatID: "chat", PeerID: "a", Gateway: 1}}}
	file := filepath.Join(t.TempDir(), "fixture.json")
	data, _ := json.Marshal(f)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c := config{fixtureFile: file, out: filepath.Join(t.TempDir(), "run"), gateways: []gateway{{"a", wsURL}, {"b", wsURL}}, users: 1, rates: []float64{80}, step: 150 * time.Millisecond, drain: 50 * time.Millisecond, mix: mix{sendMessage: 1}, seed: 1}
	rep, err := run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	for g := range 2 {
		d := rep.deliveries[key{0, g, sendMessage}]
		if d == nil || d.received == 0 || d.received != d.accepted || d.missing+d.duplicates+d.mismatches+d.reordered > 0 {
			t.Fatalf("cross delivery: %+v", d)
		}
	}
	rows := readRows(t, filepath.Join(c.out, "deliveries.csv"))
	calls := readRows(t, filepath.Join(c.out, "calls.csv"))
	senders := map[string]string{}
	for _, c := range calls {
		senders[c["operation_id"]] = c["user_id"]
	}
	for _, r := range rows {
		if senders[r["operation_id"]] == r["receiver_id"] || r["received_ns"] == "" || r["notify_ns"] == "" {
			t.Fatal("missing real recipient", r)
		}
	}
	for _, name := range []string{"calls.csv", "deliveries.csv", "meta.json"} {
		b, err := os.ReadFile(filepath.Join(c.out, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "-secret") {
			t.Fatal("credentials in raw results")
		}
	}
}

func TestFixtureRejectsNonReciprocalOrSameGateway(t *testing.T) {
	for _, users := range [][]fixtureUser{
		{{ID: "a", Token: "x", ChatID: "c", PeerID: "a", Gateway: 0}},
		{{ID: "a", Token: "x", ChatID: "c", PeerID: "b", Gateway: 0}, {ID: "b", Token: "y", ChatID: "c", PeerID: "a", Gateway: 0}},
		{{ID: "a", Token: "x", ChatID: "c", PeerID: "b", Gateway: 0}, {ID: "b", Token: "y", ChatID: "other", PeerID: "a", Gateway: 1}},
	} {
		f := filepath.Join(t.TempDir(), "fixture.json")
		b, _ := json.Marshal(fixture{Schema: 1, Users: users})
		if err := os.WriteFile(f, b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readFixture(f, 2); err == nil {
			t.Fatal("invalid pair accepted")
		}
	}
}
