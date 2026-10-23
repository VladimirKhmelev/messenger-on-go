package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
)

const secret = "test-secret"

// stack fakes chat-service and a gateway as wsload sees them: it creates
// chats, checks the tokens as the services do and answers the requests of a
// connection one by one, pushing notifications in between.
type stack struct {
	t     *testing.T
	delay time.Duration // before every answer
	// silent kinds of requests get no answer; failing ones get an error.
	silent, failing map[string]bool
	hang            atomic.Bool // no more answers at all

	mu    sync.Mutex
	chats map[string]string         // chat ID by user ID
	sends map[string]map[string]int // messages by gateway and user
}

func newStack(t *testing.T) *stack {
	return &stack{t: t, silent: map[string]bool{}, failing: map[string]bool{}, chats: map[string]string{}, sends: map[string]map[string]int{}}
}

func (s *stack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/chats":
		s.createChat(w, r)
	case "/ws":
		s.serveWS(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *stack) createChat(w http.ResponseWriter, r *http.Request) {
	userID, err := jwtutil.ValidateAccessToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), secret)
	if err != nil {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	var req struct {
		TargetUserID        string            `json:"target_user_id"`
		EncryptedChatKey    map[string]string `json:"encrypted_chat_key"`
		WrappedForPublicKey map[string]string `json:"wrapped_for_public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetUserID != userID ||
		req.EncryptedChatKey[userID] == "" || req.WrappedForPublicKey[userID] == "" {
		http.Error(w, "not a chat with oneself", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	chatID := "chat-of-" + userID
	s.chats[userID] = chatID
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"chatId": chatID})
}

func (s *stack) serveWS(w http.ResponseWriter, r *http.Request) {
	userID, err := jwtutil.ValidateAccessToken(r.URL.Query().Get("token"), secret)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = ws.Close() }()
	s.mu.Lock()
	chatID := s.chats[userID]
	s.mu.Unlock()
	write := func(v any) { _ = ws.WriteJSON(v) }
	for {
		var m clientMessage
		if err := ws.ReadJSON(&m); err != nil {
			return
		}
		time.Sleep(s.delay)
		if s.hang.Load() || s.silent[m.Type] {
			continue
		}
		if s.failing[m.Type] {
			write(map[string]string{"type": "error", "error": m.Type + " failed"})
			continue
		}
		switch m.Type {
		case "get_presence":
			if m.PeerUserID == "" || m.PeerUserID == userID {
				s.t.Errorf("get_presence asks about %q", m.PeerUserID)
			}
			// Another user's presence changes meanwhile.
			write(map[string]any{"type": "presence_changed", "peer_user_id": "someone-else", "online": true})
			write(map[string]any{"type": "presence_changed", "peer_user_id": m.PeerUserID})
		case "get_read_status":
			if m.ChatID != chatID || m.PeerUserID != userID {
				s.t.Errorf("get_read_status of %q in %q", m.PeerUserID, m.ChatID)
			}
			write(map[string]any{"type": "read_status", "chat_id": m.ChatID, "peer_user_id": userID})
		case "send_message":
			if m.ChatID != chatID || m.Text == "" {
				s.t.Errorf("send_message %q to %q", m.Text, m.ChatID)
			}
			s.mu.Lock()
			gw := r.URL.Query().Get("gateway")
			if s.sends[gw] == nil {
				s.sends[gw] = map[string]int{}
			}
			s.sends[gw][userID]++
			s.mu.Unlock()
			// The message reaches its chat before ws-gateway answers.
			write(map[string]any{"type": "message_received", "chat_id": m.ChatID, "message": map[string]string{"text": m.Text}})
			write(map[string]any{"type": "message_sent", "message_id": "m"})
		default:
			write(map[string]string{"type": "error", "error": "unknown message type: " + m.Type})
		}
	}
}

// start runs the fake stack and returns a config of wsload for it, with
// two gateways that the stack tells apart by a parameter of their URLs.
func (s *stack) start(rates ...float64) config {
	srv := httptest.NewServer(s)
	s.t.Cleanup(srv.Close)
	ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?gateway="
	m, err := parseMix("get_presence=1,get_read_status=1,send_message=1")
	if err != nil {
		s.t.Fatal(err)
	}
	return config{
		gateways: []gateway{{"a", ws + "a"}, {"b", ws + "b"}}, chatURL: srv.URL, secret: secret, users: 5,
		rates: rates, step: 500 * time.Millisecond, warmup: 200 * time.Millisecond, drain: time.Second, mix: m, seed: 1,
	}
}

// TestRun loads the fake stack: every request sent in a step is answered,
// after the delay of the stack, and the notifications in between are not
// taken for answers.
func TestRun(t *testing.T) {
	s := newStack(t)
	s.delay = 2 * time.Millisecond
	c := s.start(60, 120)
	rep, err := run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rep.strays != 0 || len(rep.errors) != 0 {
		t.Errorf("%d strays, errors %v", rep.strays, rep.errors)
	}
	for i, rate := range c.rates {
		for g := range c.gateways {
			sent := 0
			for k := range kinds {
				tl := rep.tallies[key{i, g, k}]
				if tl == nil {
					t.Fatalf("step %d, gateway %d: no %s", i, g, kindNames[k])
				}
				if tl.ok != tl.sent || tl.failed != 0 || tl.lost != 0 || len(tl.latencies) != tl.ok {
					t.Errorf("step %d, gateway %d, %s: %+v", i, g, kindNames[k], *tl)
				}
				for _, l := range tl.latencies {
					if l < s.delay {
						t.Fatalf("latency %v under the delay of the stack", l)
					}
				}
				sent += tl.sent
			}
			// Half the rate a gateway for half a second: a Poisson count
			// around rate/4.
			if want := rate / 4; math.Abs(float64(sent)-want) > 4*math.Sqrt(want) {
				t.Errorf("step %d, gateway %d: %d requests, want about %.0f", i, g, sent, want)
			}
		}
	}
	// The users of a gateway take turns to send, to keep under the limit
	// of chat-service.
	for _, gw := range []string{"a", "b"} {
		counts := slices.Collect(maps.Values(s.sends[gw]))
		if len(counts) != c.users || slices.Max(counts)-slices.Min(counts) > 1 {
			t.Errorf("gateway %s: messages by user %v", gw, counts)
		}
	}
}

// TestRunErrorsAndLosses: an error answers its request, and the requests
// still without an answer at the end count as lost.
func TestRunErrorsAndLosses(t *testing.T) {
	s := newStack(t)
	s.failing["send_message"] = true
	c := s.start(100, 100)
	c.drain = 100 * time.Millisecond
	// The stack stops answering halfway through the second step; the setup
	// takes a few milliseconds.
	hang := time.AfterFunc(c.warmup+c.step+c.step/2, func() { s.hang.Store(true) })
	defer hang.Stop()
	rep, err := run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	for g := range c.gateways {
		lost := 0
		for step := range c.rates {
			for k := range kinds {
				tl := rep.tallies[key{step, g, k}]
				if tl.ok+tl.failed+tl.lost != tl.sent || (k == sendMessage) != (tl.failed > 0) || (k == sendMessage && tl.ok > 0) {
					t.Errorf("step %d, gateway %d, %s: %+v", step, g, kindNames[k], *tl)
				}
				if step == 0 && tl.lost > 0 {
					t.Errorf("gateway %d, %s: %d lost before the stack hung", g, kindNames[k], tl.lost)
				}
				lost += tl.lost
			}
		}
		if lost == 0 {
			t.Errorf("gateway %d: nothing lost after the stack hung", g)
		}
	}
	if rep.errors["send_message failed"] == 0 || len(rep.errors) != 1 || rep.strays != 0 {
		t.Errorf("errors %v, %d strays", rep.errors, rep.strays)
	}
}

// TestRunMismatch: an answer of another kind than its request is an error
// and says so.
func TestRunMismatch(t *testing.T) {
	s := newStack(t)
	s.silent["get_presence"] = true
	c := s.start(100)
	c.mix = mix{getPresence: 0.5, sendMessage: 0.5}
	c.drain = 100 * time.Millisecond
	rep, err := run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rep.errors["get_presence answered with message_sent"] == 0 {
		t.Errorf("a message_sent to a get_presence went unnoticed: errors %v", rep.errors)
	}
}

func TestAnswer(t *testing.T) {
	for _, tc := range []struct {
		m        serverMessage
		k        kind
		isAnswer bool
	}{
		{serverMessage{Type: "error", Error: "x"}, anyKind, true},
		{serverMessage{Type: "message_sent"}, sendMessage, true},
		{serverMessage{Type: "read_status"}, getReadStatus, true},
		{serverMessage{Type: "presence_changed", PeerUserID: "idle"}, getPresence, true},
		{serverMessage{Type: "presence_changed", PeerUserID: "someone"}, getPresence, false},
		{serverMessage{Type: "message_received"}, 0, false},
		{serverMessage{Type: "notify_push"}, 0, false},
	} {
		if k, ok := answer(tc.m, "idle"); ok != tc.isAnswer || (ok && k != tc.k) {
			t.Errorf("answer(%+v) = %v, %v, want %v, %v", tc.m, k, ok, tc.k, tc.isAnswer)
		}
	}
}

func TestParseMix(t *testing.T) {
	m, err := parseMix("get_presence=45,get_read_status=45,send_message=10")
	if err != nil || m != (mix{0.45, 0.45, 0.1}) {
		t.Errorf("parseMix = %v, %v", m, err)
	}
	if m, err := parseMix("send_message=2,send_message=2"); err != nil || m != (mix{sendMessage: 1}) {
		t.Errorf("a repeated request: %v, %v", m, err)
	}
	for _, bad := range []string{"", "get_history=1", "send_message", "send_message=-1", "send_message=x", "send_message=0"} {
		if _, err := parseMix(bad); err == nil {
			t.Errorf("parseMix(%q) succeeded", bad)
		}
	}
}

func TestPick(t *testing.T) {
	m := mix{0.25, 0, 0.75}
	for _, tc := range []struct {
		x float64
		k kind
	}{{0, getPresence}, {0.2499, getPresence}, {0.25, sendMessage}, {0.9999, sendMessage}} {
		if k := m.pick(tc.x); k != tc.k {
			t.Errorf("pick(%v) = %v, want %v", tc.x, k, tc.k)
		}
	}
	// Rounding never leaves x past the last share.
	if k := (mix{0.1, 0.2, 0.7}).pick(math.Nextafter(1, 0)); k != sendMessage {
		t.Errorf("pick near 1 = %v", k)
	}
}

func TestSchedule(t *testing.T) {
	m := mix{0.5, 0.25, 0.25}
	plan := func(seed uint64) []arrival {
		return schedule(rand.New(rand.NewPCG(seed, 0)), []float64{100, 400}, 10*time.Second, 20*time.Second, m)
	}
	a := plan(1)
	if b := plan(1); len(a) != len(b) || a[len(a)-1] != b[len(b)-1] {
		t.Error("the same seed gave another schedule")
	}
	if b := plan(2); len(b) == len(a) && b[0] == a[0] {
		t.Error("another seed gave the same schedule")
	}
	counts := map[int]float64{}
	kinds := map[kind]float64{}
	for i, x := range a {
		if i > 0 && x.at < a[i-1].at {
			t.Fatal("the arrivals go back in time")
		}
		wantStep := -1
		switch {
		case x.at >= 30*time.Second:
			wantStep = 1
		case x.at >= 10*time.Second:
			wantStep = 0
		}
		if x.step != wantStep || x.at >= 50*time.Second {
			t.Fatalf("an arrival at %v in step %d", x.at, x.step)
		}
		counts[x.step]++
		kinds[x.kind]++
	}
	for step, want := range map[int]float64{-1: 1000, 0: 2000, 1: 8000} {
		if math.Abs(counts[step]-want) > 4*math.Sqrt(want) {
			t.Errorf("step %d: %v arrivals, want about %v", step, counts[step], want)
		}
	}
	total := float64(len(a))
	for k, share := range m {
		if want := share * total; math.Abs(kinds[kind(k)]-want) > 4*math.Sqrt(want) {
			t.Errorf("%s: %v, want about %.0f", kindNames[k], kinds[kind(k)], want)
		}
	}
}

func TestParseFlags(t *testing.T) {
	c, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.gateways) != 2 || c.gateways[1] != (gateway{"ws-gateway-2", "ws://localhost:8081/ws"}) || len(c.rates) != 3 || c.users != 200 {
		t.Errorf("defaults: %+v", c)
	}
	c, err = parseFlags([]string{"-gateways", "ws://a/ws,quic=wss://b/ws", "-steps", "10", "-chat", "http://chat/"})
	if err != nil || c.gateways[0] != (gateway{"gateway-1", "ws://a/ws"}) || c.gateways[1].name != "quic" || c.chatURL != "http://chat" {
		t.Errorf("parseFlags = %+v, %v", c, err)
	}
	for _, bad := range [][]string{
		{"-gateways", "http://a/ws"}, {"-gateways", "ws://"}, {"-steps", "10,0"}, {"-steps", "x"},
		{"-users", "0"}, {"-step", "0s"}, {"-mix", "nothing=1"}, {"extra"},
		// 500 rps on two gateways, 10% sends over 50 users each: 30 a
		// minute per user.
		{"-steps", "500", "-users", "50"},
	} {
		if _, err := parseFlags(bad); err == nil {
			t.Errorf("parseFlags(%q) succeeded", bad)
		}
	}
	// 48 users each send 500/2 × 10% × 60 / 48 ≈ 31 a minute; 60 send 25.
	if _, err := parseFlags([]string{"-steps", "500", "-users", "60"}); err != nil {
		t.Errorf("within the budget: %v", err)
	}
}

func TestSignToken(t *testing.T) {
	tok, err := signToken(secret, "user", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if id, err := jwtutil.ValidateAccessToken(tok, secret); err != nil || id != "user" {
		t.Errorf("the services read %q, %v", id, err)
	}
	expired, _ := signToken(secret, "user", time.Now().Add(-time.Minute))
	if _, err := jwtutil.ValidateAccessToken(expired, secret); err == nil {
		t.Error("an expired token passed")
	}
}

func TestNewID(t *testing.T) {
	a, b := newID(), newID()
	if a == b || len(a) != 36 || a[14] != '4' || !strings.ContainsRune("89ab", rune(a[19])) {
		t.Errorf("newID = %q, %q", a, b)
	}
}

func TestCreateChatFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"no"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := createChat(t.Context(), srv.URL, user{id: "u"}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("createChat = %v", err)
	}
}

func TestQuantile(t *testing.T) {
	ms := func(n ...int) []time.Duration {
		var out []time.Duration
		for _, x := range n {
			out = append(out, time.Duration(x)*time.Millisecond)
		}
		return out
	}
	for _, tc := range []struct {
		l    []time.Duration
		q    float64
		want string
	}{
		{nil, 0.5, "-"}, {ms(7), 0.99, "7.00"}, {ms(1, 2, 3, 4), 0.5, "2.00"}, {ms(1, 2, 3, 4), 0.51, "3.00"},
		{ms(1, 2, 3, 4, 5, 6, 7, 8, 9, 10), 0.99, "10.00"}, {ms(1, 2, 3, 4, 5, 6, 7, 8, 9, 10), 0.9, "9.00"},
	} {
		if got := quantile(tc.l, tc.q); got != tc.want {
			t.Errorf("quantile(%v, %v) = %s, want %s", tc.l, tc.q, got, tc.want)
		}
	}
}

func TestPrint(t *testing.T) {
	c := config{gateways: []gateway{{"grpc", ""}}, rates: []float64{10}, step: 10 * time.Second}
	rep := &report{c: c, start: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), tallies: map[key]*tally{
		{0, 0, getPresence}: {sent: 3, ok: 2, lost: 1, latencies: []time.Duration{time.Millisecond, 3 * time.Millisecond}},
		{0, 0, sendMessage}: {sent: 1, failed: 1},
	}, errors: map[string]int{"b": 1, "a": 1, "c": 5}, strays: 2}
	var b bytes.Buffer
	rep.print(&b)
	out := b.String()
	for _, want := range []string{
		"1     10   03:04:05    grpc     get_presence  3     2   0       1     0.2   1.00    3.00",
		"send_message  1     0   1       0     0.0   -       -",
		"all           4     2   1       1     0.2   1.00    3.00",
		"       5  c\n       1  a\n       1  b\n",
		"2 answers matched no request",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "get_read_status") {
		t.Errorf("a row for a request never sent:\n%s", out)
	}
}
