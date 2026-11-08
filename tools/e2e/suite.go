package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type deliveryResult struct {
	Missing    int `json:"missing"`
	Duplicates int `json:"duplicates"`
	OutOfOrder int `json:"out_of_order"`
}
type caseResult struct {
	Name       string         `json:"name"`
	Outcome    string         `json:"outcome"`
	DurationMS int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}
type report struct {
	Prepared  prepared     `json:"prepared"`
	Schema    int          `json:"schema"`
	Run       string       `json:"run"`
	Revision  string       `json:"revision"`
	Transport string       `json:"transport"`
	Started   time.Time    `json:"started"`
	Finished  *time.Time   `json:"finished,omitempty"`
	Passed    bool         `json:"passed"`
	Cases     []caseResult `json:"cases"`
}

func (r *report) save(dir string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "report.json.tmp")
	if err = os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "report.json"))
}
func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

type endpoints struct {
	auth, chat, media, worker, mail, nats, minio string
	gateways                                     [2]string
}
type user struct{ ID, token, email string }
type suite struct {
	api *api
	endpoints
	control         *compose
	rep             *report
	dir             string
	users           [3]user
	sockets         [3]*socket
	chatID          string
	mediaID         string
	observe         time.Duration
	uncertainWrites map[string]string
}

func (s *suite) ready(ctx context.Context) error {
	for addr, path := range map[string]string{s.auth: "/v1/auth/health", s.chat: "/v1/chat/health", s.media: "/v1/media/health"} {
		if err := poll(ctx, func() error {
			var health struct {
				OK bool `json:"ok"`
			}
			if err := s.api.json(ctx, "GET", addr+path, "", nil, &health); err != nil {
				return err
			}
			if !health.OK {
				return fmt.Errorf("service not healthy")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Consumers must exist before publishing: the gateways deliberately use DeliverNew.
func (s *suite) consumersReady(ctx context.Context) error {
	return poll(ctx, func() error {
		var monitor struct {
			Accounts []struct {
				Streams []struct {
					Name      string
					Consumers []struct {
						Config struct {
							Durable string   `json:"durable_name"`
							Filters []string `json:"filter_subjects"`
							Filter  string   `json:"filter_subject"`
						}
					} `json:"consumer_detail"`
				} `json:"stream_detail"`
			} `json:"account_details"`
		}
		if err := s.api.json(ctx, "GET", s.nats+"/jsz?accounts=true&streams=true&consumers=true&config=true", "", nil, &monitor); err != nil {
			return err
		}
		chats, notify, profile, worker := 0, 0, 0, false
		for _, account := range monitor.Accounts {
			for _, stream := range account.Streams {
				for _, consumer := range stream.Consumers {
					cfg := consumer.Config
					switch stream.Name {
					case "CHAT_EVENTS":
						if len(cfg.Filters) == 5 && slices.Contains(cfg.Filters, "chat.deleted") {
							chats++
						}
						if cfg.Durable == "notification-worker-chat-events" {
							worker = true
						}
					case "NOTIFY_EVENTS":
						if cfg.Filter == "notify.push" || slices.Contains(cfg.Filters, "notify.push") {
							notify++
						}
					case "USER_EVENTS":
						if cfg.Filter == "user.profile_updated" || slices.Contains(cfg.Filters, "user.profile_updated") {
							profile++
						}
					}
				}
			}
		}
		if chats != 2 || notify != 2 || profile != 2 || !worker {
			return fmt.Errorf("consumers not ready: chat=%d notify=%d profile=%d worker=%v", chats, notify, profile, worker)
		}
		return nil
	})
}

func (s *suite) register(ctx context.Context, index int) (user, error) {
	email := fmt.Sprintf("e2e-%s-%d@example.test", s.rep.Run, index)
	tag := fmt.Sprintf("e%s%d", s.rep.Run, index)
	var registered struct {
		ID string `json:"userId"`
	}
	err := s.api.json(ctx, "POST", s.auth+"/v1/auth/register", "", map[string]any{"email": email, "password": "E2ePassword123!", "tag": tag, "display_name": fmt.Sprintf("E2E %d", index), "public_key": "e2e-public-key", "wrapped_private_key": "e2e-wrapped-key", "key_wrap_salt": "e2e-salt"}, &registered)
	if err != nil {
		return user{}, err
	}
	if registered.ID == "" {
		return user{}, fmt.Errorf("register returned no ID")
	}
	var code string
	err = poll(ctx, func() error {
		var box json.RawMessage
		if err := s.api.json(ctx, "GET", s.mail+"/api/v2/messages?start=0&limit=100", "", nil, &box); err != nil {
			return err
		}
		var err error
		code, err = mailCode(box, email)
		return err
	})
	if err != nil {
		return user{}, err
	}
	if err = s.api.json(ctx, "POST", s.auth+"/v1/auth/verify-email", "", map[string]string{"email": email, "code": code}, nil); err != nil {
		return user{}, err
	}
	var login struct {
		Token string `json:"accessToken"`
	}
	if err = s.api.json(ctx, "POST", s.auth+"/v1/auth/login", "", map[string]string{"email": email, "password": "E2ePassword123!"}, &login); err != nil {
		return user{}, err
	}
	if login.Token == "" {
		return user{}, fmt.Errorf("login returned no access token")
	}
	return user{ID: registered.ID, token: login.Token, email: email}, nil
}

func (s *suite) chatBody(group bool) map[string]any {
	keys := map[string]string{s.users[0].ID: "e2e-key-a", s.users[1].ID: "e2e-key-b"}
	public := map[string]string{s.users[0].ID: "e2e-public-key", s.users[1].ID: "e2e-public-key"}
	body := map[string]any{"encrypted_chat_key": keys, "wrapped_for_public_key": public}
	if group {
		keys[s.users[2].ID] = "e2e-key-c"
		public[s.users[2].ID] = "e2e-public-key"
		body["name"] = "E2E group"
		body["target_user_ids"] = []string{s.users[1].ID, s.users[2].ID}
	} else {
		body["target_user_id"] = s.users[1].ID
	}
	return body
}
func (s *suite) createChat(ctx context.Context, group bool) (string, error) {
	path := "/v1/chats"
	if group {
		path += "/group"
	}
	var result struct {
		ID string `json:"chatId"`
	}
	if err := s.api.json(ctx, "POST", s.chat+path, s.users[0].token, s.chatBody(group), &result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("create chat returned no ID")
	}
	return result.ID, nil
}

func (s *suite) setup(ctx context.Context, _ *caseResult) error {
	for i := range s.users {
		u, err := s.register(ctx, i)
		if err != nil {
			return err
		}
		s.users[i] = u
	}
	id, err := s.createChat(ctx, false)
	if err != nil {
		return err
	}
	s.chatID = id
	for i, u := range s.users {
		gateway := s.gateways[0]
		if i == 1 {
			gateway = s.gateways[1]
		}
		sock, err := dialSocket(ctx, gateway, u.token)
		if err != nil {
			return err
		}
		s.sockets[i] = sock
	}
	// A server-side registry metric removes the race between Upgrade and Add.
	for i, gw := range s.gateways {
		want := float64(2)
		if i == 1 {
			want = 1
		}
		if err := poll(ctx, func() error {
			text, err := s.metrics(ctx, gw)
			if err != nil {
				return err
			}
			if !gaugeEquals(text, "ws_connections_active", want) {
				return fmt.Errorf("WebSocket registry not ready")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
func gaugeEquals(text, name string, want float64) bool {
	var got float64
	for _, line := range bytes.Split([]byte(text), []byte("\n")) {
		if _, err := fmt.Sscanf(string(line), name+" %f", &got); err == nil && got == want {
			return true
		}
	}
	return false
}
func (s *suite) metrics(ctx context.Context, addr string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", addr+"/metrics", nil)
	if err != nil {
		return "", err
	}
	resp, err := s.api.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("metrics: HTTP %d", resp.StatusCode)
	}
	data, err := readBody(resp.Body)
	return string(data), err
}

func (s *suite) unauthorized(ctx context.Context, _ *caseResult) error {
	for _, addr := range []string{s.chat + "/v1/chats", s.media + "/v1/media/uploads"} {
		method := "GET"
		var body any
		if addr == s.media+"/v1/media/uploads" {
			method = "POST"
			body = map[string]any{}
		}
		code, _ := s.api.request(ctx, method, addr, "", body, nil)
		if code != http.StatusUnauthorized {
			return fmt.Errorf("unauthenticated request: HTTP %d", code)
		}
	}
	code, _ := s.api.request(ctx, "GET", s.chat+"/v1/chats/"+s.chatID+"/messages", s.users[2].token, nil, nil)
	if code != http.StatusForbidden {
		return fmt.Errorf("non-member history: HTTP %d", code)
	}
	code, _ = s.api.request(ctx, "POST", s.media+"/v1/media/uploads", s.users[2].token, map[string]any{"chat_id": s.chatID, "content_type": "text/plain", "size_bytes": 4}, nil)
	if code != http.StatusForbidden {
		return fmt.Errorf("non-member upload: HTTP %d", code)
	}
	return nil
}

func (s *suite) send(ctx context.Context, from int, text string) (string, error) {
	marks := [2]int{s.sockets[0].mark(), s.sockets[1].mark()}
	id, err := s.sockets[from].ack(ctx, s.chatID, text)
	if err != nil {
		return "", err
	}
	for i := 0; i < 2; i++ {
		if err = s.sockets[i].receipt(ctx, marks[i], s.chatID, id, text, s.users[from].ID); err != nil {
			return id, err
		}
	}
	return id, nil
}

func (s *suite) history(ctx context.Context, who int) ([]message, error) {
	var response struct {
		Messages []struct {
			ID      string `json:"messageId"`
			Sender  string `json:"senderUserId"`
			Text    string `json:"text"`
			Deleted bool   `json:"deleted"`
		}
	}
	err := s.api.json(ctx, "GET", s.chat+"/v1/chats/"+s.chatID+"/messages?limit=100", s.users[who].token, nil, &response)
	out := make([]message, 0, len(response.Messages))
	for _, m := range response.Messages {
		out = append(out, message{ID: m.ID, Sender: m.Sender, Text: m.Text, Deleted: m.Deleted})
	}
	return out, err
}
func (s *suite) quiet(ctx context.Context) error {
	timer := time.NewTimer(s.observe)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (s *suite) delivery(ctx context.Context, result *caseResult) error {
	marks := [3]int{s.sockets[0].mark(), s.sockets[1].mark(), s.sockets[2].mark()}
	var ids []string
	for i := range 3 {
		id, err := s.send(ctx, i%2, fmt.Sprintf("e2e:%s:%d", s.rep.Run, i))
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	// Connected without set_active is intentionally offline for notification-worker.
	if _, err := s.sockets[1].wait(ctx, marks[1], func(e event) bool { return e.Type == "notify_push" && e.ID == ids[0] && e.Chat == s.chatID }); err != nil {
		return fmt.Errorf("notification-worker delivery: %w", err)
	}
	if err := s.quiet(ctx); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		missing, dups, order := inspectDelivery(s.sockets[i].snapshot(marks[i]), s.chatID, ids)
		result.Details[fmt.Sprintf("gateway_%d", i+1)] = deliveryResult{missing, dups, order}
		if missing+dups+order != 0 {
			return fmt.Errorf("gateway %d: missing=%d duplicates=%d out_of_order=%d", i+1, missing, dups, order)
		}
	}
	for _, e := range s.sockets[2].snapshot(marks[2]) {
		if e.Chat == s.chatID {
			return fmt.Errorf("non-member received chat event")
		}
	}
	history, err := s.history(ctx, 1)
	if err != nil {
		return err
	}
	if len(history) != 3 {
		return fmt.Errorf("history has %d messages, want 3", len(history))
	}
	for i, m := range history {
		if m.Text != fmt.Sprintf("e2e:%s:%d", s.rep.Run, i) || m.Sender != s.users[i%2].ID {
			return fmt.Errorf("history content or sender differs at %d", i)
		}
		if m.ID != ids[i] {
			return fmt.Errorf("history order differs at %d", i)
		}
	}
	return nil
}

func (s *suite) mutations(ctx context.Context, _ *caseResult) error {
	history, err := s.history(ctx, 0)
	if err != nil || len(history) == 0 {
		return fmt.Errorf("history unavailable before mutations: %w", err)
	}
	id := history[0].ID
	mark := s.sockets[1].mark()
	if err = s.sockets[0].send(ctx, map[string]any{"type": "edit_message", "chat_id": s.chatID, "message_id": id, "text": "e2e-edited"}); err != nil {
		return err
	}
	if _, err = s.sockets[1].wait(ctx, mark, func(e event) bool {
		return e.Type == "message_updated" && e.ID == id && e.NewText != nil && *e.NewText == "e2e-edited"
	}); err != nil {
		return err
	}
	mark = s.sockets[0].mark()
	if err = s.sockets[1].send(ctx, map[string]any{"type": "mark_read", "chat_id": s.chatID, "message_id": id}); err != nil {
		return err
	}
	if _, err = s.sockets[0].wait(ctx, mark, func(e event) bool { return e.Type == "read_status" && e.LastRead == id && e.Peer == s.users[1].ID }); err != nil {
		return err
	}
	history, err = s.history(ctx, 1)
	if err != nil {
		return err
	}
	if history[0].Text != "e2e-edited" {
		return fmt.Errorf("edit not visible in history")
	}
	mark = s.sockets[1].mark()
	if err = s.sockets[0].send(ctx, map[string]any{"type": "delete_message_for_all", "chat_id": s.chatID, "message_id": id}); err != nil {
		return err
	}
	if _, err = s.sockets[1].wait(ctx, mark, func(e event) bool { return e.Type == "message_updated" && e.ID == id && e.Deleted }); err != nil {
		return err
	}
	history, err = s.history(ctx, 1)
	if err != nil {
		return err
	}
	if !history[0].Deleted {
		return fmt.Errorf("deletion not visible in history")
	}
	group, err := s.createChat(ctx, true)
	if err != nil {
		return err
	}
	mark = s.sockets[1].mark()
	if err = s.api.json(ctx, "DELETE", s.chat+"/v1/chats/"+group, s.users[0].token, nil, nil); err != nil {
		return err
	}
	_, err = s.sockets[1].wait(ctx, mark, func(e event) bool { return e.Type == "chat_deleted" && e.Chat == group })
	return err
}

func (s *suite) mediaRoundTrip(ctx context.Context, _ *caseResult) error {
	data := []byte("e2e opaque attachment: " + s.rep.Run)
	var ticket struct {
		ID  string `json:"uploadId"`
		URL string `json:"uploadUrl"`
	}
	err := s.api.json(ctx, "POST", s.media+"/v1/media/uploads", s.users[0].token, map[string]any{"chat_id": s.chatID, "content_type": "application/octet-stream", "size_bytes": len(data)}, &ticket)
	if err != nil {
		return err
	}
	if ticket.ID == "" {
		return fmt.Errorf("upload has no ID")
	}
	if _, err = s.api.object(ctx, "PUT", ticket.URL, data); err != nil {
		return err
	}
	var confirmed struct {
		ID string `json:"mediaId"`
	}
	if err = s.api.json(ctx, "POST", s.media+"/v1/media/uploads/"+ticket.ID+"/confirm", s.users[0].token, map[string]any{}, &confirmed); err != nil {
		return err
	}
	s.mediaID = confirmed.ID
	if confirmed.ID == "" {
		return fmt.Errorf("confirmed media has no ID")
	}
	var download struct {
		URL string `json:"downloadUrl"`
	}
	if err = s.api.json(ctx, "GET", s.media+"/v1/media/"+confirmed.ID+"/download-url", s.users[1].token, nil, &download); err != nil {
		return err
	}
	got, err := s.api.object(ctx, "GET", download.URL, nil)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, got) {
		return fmt.Errorf("downloaded attachment differs")
	}
	code, _ := s.api.request(ctx, "GET", s.media+"/v1/media/"+confirmed.ID+"/download-url", s.users[2].token, nil, nil)
	if code != http.StatusForbidden {
		return fmt.Errorf("non-member download: HTTP %d", code)
	}
	return nil
}

func (s *suite) edges(ctx context.Context, result *caseResult) error {
	for _, edge := range []struct{ endpoint, service, method string }{
		{s.gateways[0], "ws-gateway", "/chat.v1.ChatService/SendMessage"}, {s.gateways[1], "ws-gateway", "/chat.v1.ChatService/SendMessage"},
		{s.chat, "chat-service", "/auth.v1.AuthService/GetUserByID"}, {s.media, "media-service", "/chat.v1.ChatService/ListMembers"},
		{s.worker, "notification-worker", "/chat.v1.ChatService/ListMembers"}, {s.worker, "notification-worker", "/auth.v1.AuthService/ListPushSubscriptions"},
	} {
		var count float64
		if err := poll(ctx, func() error {
			text, err := s.metrics(ctx, edge.endpoint)
			if err != nil {
				return err
			}
			count = rpcCount(text, edge.service, s.rep.Transport, edge.method)
			if count <= 0 {
				return fmt.Errorf("no successful %s %s over %s", edge.service, edge.method, s.rep.Transport)
			}
			return nil
		}); err != nil {
			return err
		}
		result.Details[edge.endpoint+edge.method] = count
	}
	return nil
}

func (s *suite) run(ctx context.Context) error {
	tests := []struct {
		name string
		run  func(context.Context, *caseResult) error
	}{
		{"registration_login_and_cross_gateway_setup", s.setup}, {"authorization_boundaries", s.unauthorized}, {"cross_gateway_delivery_history_notifications", s.delivery},
		{"edit_read_delete_and_group_delete", s.mutations}, {"media_upload_download_and_access", s.mediaRoundTrip}, {"all_interservice_edges_use_selected_transport", s.edges},
		{"delivery_with_delay_and_loss", s.delayLoss}, {"deadline_and_cancellation", s.cancellation}, {"blackhole_and_uncertain_write", s.blackhole},
		{"auth_graceful_restart", s.restartCase("auth-service", false)}, {"auth_crash_restart", s.restartCase("auth-service", true)},
		{"chat_graceful_restart", s.restartCase("chat-service", false)}, {"chat_crash_restart", s.restartCase("chat-service", true)},
		{"final_history_audit_and_client_liveness", s.finalRecovery},
	}
	for _, test := range tests {
		s.rep.Cases = append(s.rep.Cases, caseResult{Name: test.name, Outcome: "not_run"})
	}
	defer func() {
		for _, sock := range s.sockets {
			if sock != nil {
				sock.close()
			}
		}
	}()
	if err := s.rep.save(s.dir); err != nil {
		return err
	}
	for i, test := range tests {
		result := &s.rep.Cases[i]
		result.Details = map[string]any{}
		started := time.Now()
		bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := test.run(bounded, result)
		cancel()
		result.DurationMS = time.Since(started).Milliseconds()
		result.Outcome = "passed"
		if err != nil {
			result.Outcome = "failed"
			result.Error = err.Error()
		}
		if saveErr := s.rep.save(s.dir); saveErr != nil {
			return saveErr
		}
		fmt.Printf("%s: %s\n", result.Outcome, test.name)
		if err != nil {
			return fmt.Errorf("%s: %w", test.name, err)
		}
	}
	return nil
}

func minioAddress(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func (s *suite) audit(ctx context.Context, result *caseResult) error {
	for i := 0; i < 2; i++ {
		if _, err := s.send(ctx, i, "final-liveness:"+fmt.Sprint(i)); err != nil {
			return err
		}
	}
	if err := s.quiet(ctx); err != nil {
		return err
	}
	history, err := s.history(ctx, 1)
	if err != nil {
		return err
	}
	for text, ackID := range s.uncertainWrites {
		var ids []string
		for _, m := range history {
			if m.Text == text {
				ids = append(ids, m.ID)
			}
		}
		result.Details[text] = len(ids)
		if len(ids) > 1 {
			return fmt.Errorf("late duplicate stored write: %s", text)
		}
		if ackID != "" && (len(ids) != 1 || ids[0] != ackID) {
			return fmt.Errorf("acknowledged write disappeared: %s", text)
		}
	}
	return nil
}

func (s *suite) finalRecovery(ctx context.Context, result *caseResult) error {
	if err := s.audit(ctx, result); err != nil {
		return err
	}
	// Exercise the unchanged media client after both chat restarts. Poll a
	// read, then perform a new upload exactly once after it has reconnected.
	if s.mediaID == "" {
		return fmt.Errorf("no attachment available for recovery probe")
	}
	if err := poll(ctx, func() error {
		return s.api.json(ctx, "GET", s.media+"/v1/media/"+s.mediaID+"/download-url", s.users[1].token, nil, nil)
	}); err != nil {
		return err
	}
	if err := s.mediaRoundTrip(ctx, result); err != nil {
		return fmt.Errorf("media did not recover: %w", err)
	}
	beforeText, err := s.metrics(ctx, s.worker)
	if err != nil {
		return err
	}
	method := "/auth.v1.AuthService/ListPushSubscriptions"
	before := rpcCount(beforeText, "notification-worker", s.rep.Transport, method)
	for from := 0; from < 2; from++ {
		recipient := 1 - from
		mark := s.sockets[recipient].mark()
		id, err := s.send(ctx, from, "final-notify:"+fmt.Sprint(from))
		if err != nil {
			return err
		}
		if _, err = s.sockets[recipient].wait(ctx, mark, func(e event) bool { return e.Type == "notify_push" && e.Chat == s.chatID && e.ID == id }); err != nil {
			return fmt.Errorf("worker did not recover: %w", err)
		}
	}
	var delta float64
	if err := poll(ctx, func() error {
		text, err := s.metrics(ctx, s.worker)
		if err != nil {
			return err
		}
		delta = rpcCount(text, "notification-worker", s.rep.Transport, method) - before
		if delta <= 0 {
			return fmt.Errorf("no successful worker -> auth call after restarts")
		}
		return nil
	}); err != nil {
		return err
	}
	result.Details["worker_auth_success_after_restarts"] = delta
	return nil
}
