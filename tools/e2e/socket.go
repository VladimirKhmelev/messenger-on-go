package main

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type message struct {
	ID      string `json:"message_id"`
	Sender  string `json:"sender_user_id"`
	Text    string `json:"text"`
	Deleted bool   `json:"deleted"`
}

type event struct {
	Type     string   `json:"type"`
	Error    string   `json:"error"`
	Chat     string   `json:"chat_id"`
	ID       string   `json:"message_id"`
	Peer     string   `json:"peer_user_id"`
	LastRead string   `json:"last_read_message_id"`
	NewText  *string  `json:"new_text"`
	Deleted  bool     `json:"deleted"`
	Message  *message `json:"message"`
}

type socket struct {
	ws      *websocket.Conn
	writes  sync.Mutex
	mu      sync.Mutex
	events  []event
	changed chan struct{}
	err     error
	done    chan struct{}
}

func dialSocket(ctx context.Context, addr, token string) (*socket, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	u.Scheme = "ws"
	u.Path = "/ws"
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, resp, err := d.DialContext(ctx, u.String(), nil)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("WebSocket connect: %w", err)
	}
	s := &socket{ws: ws, changed: make(chan struct{}), done: make(chan struct{})}
	ws.SetReadLimit(maxBody)
	go s.read()
	return s, nil
}

func (s *socket) read() {
	defer close(s.done)
	for {
		var e event
		err := s.ws.ReadJSON(&e)
		s.mu.Lock()
		if err == nil && len(s.events) >= 2048 {
			err = fmt.Errorf("event observation buffer is full")
		}
		if err != nil {
			s.err = err
		} else {
			s.events = append(s.events, e)
		}
		close(s.changed)
		s.changed = make(chan struct{})
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}
func (s *socket) close()    { _ = s.ws.Close(); <-s.done }
func (s *socket) mark() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.events) }
func (s *socket) snapshot(from int) []event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]event(nil), s.events[from:]...)
}
func (s *socket) send(ctx context.Context, m map[string]any) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := s.ws.SetWriteDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = s.ws.Close() })
	defer stop()
	return s.ws.WriteJSON(m)
}
func (s *socket) wait(ctx context.Context, from int, match func(event) bool) (event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return event{}, err
		}
		s.mu.Lock()
		for _, e := range s.events[from:] {
			if match(e) {
				s.mu.Unlock()
				return e, nil
			}
		}
		err, changed := s.err, s.changed
		s.mu.Unlock()
		if err != nil {
			return event{}, fmt.Errorf("WebSocket closed while waiting: %w", err)
		}
		select {
		case <-ctx.Done():
			return event{}, ctx.Err()
		case <-changed:
		}
	}
}
func (s *socket) ack(ctx context.Context, chat, text string) (string, error) {
	from := s.mark()
	if err := s.send(ctx, map[string]any{"type": "send_message", "chat_id": chat, "text": text}); err != nil {
		return "", err
	}
	e, err := s.wait(ctx, from, func(e event) bool { return e.Type == "message_sent" || e.Type == "error" })
	if err != nil {
		return "", err
	}
	if e.Type == "error" {
		return "", fmt.Errorf("send_message: %s", e.Error)
	}
	if e.ID == "" {
		return "", fmt.Errorf("message_sent has no message_id")
	}
	return e.ID, nil
}
func (s *socket) receipt(ctx context.Context, from int, chat, id, text, sender string) error {
	e, err := s.wait(ctx, from, func(e event) bool {
		return e.Type == "message_received" && e.Chat == chat && e.Message != nil && e.Message.ID == id
	})
	if err != nil {
		return err
	}
	if e.Message.Sender != sender {
		return fmt.Errorf("message sender differs for %s", id)
	}
	if e.Message.Text != text {
		return fmt.Errorf("message content differs for %s", id)
	}
	return nil
}

// inspectDelivery checks first receipt order and counts extra or missing receipts.
func inspectDelivery(events []event, chat string, ids []string) (missing, duplicates, reordered int) {
	positions := map[string]int{}
	for i, id := range ids {
		positions[id] = i
	}
	seen := map[string]bool{}
	last := -1
	for _, e := range events {
		if e.Type != "message_received" || e.Chat != chat || e.Message == nil {
			continue
		}
		p, ok := positions[e.Message.ID]
		if !ok {
			continue
		}
		if seen[e.Message.ID] {
			duplicates++
			continue
		}
		seen[e.Message.ID] = true
		if p < last {
			reordered++
		}
		last = max(last, p)
	}
	return len(ids) - len(seen), duplicates, reordered
}
