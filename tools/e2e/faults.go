package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *suite) withFault(ctx context.Context, service, profile string, work func() error) (err error) {
	if err = s.control.impair(ctx, service, profile); err != nil {
		return err
	}
	defer func() {
		clean, cancel := cleanupContext()
		defer cancel()
		err = errors.Join(err, s.control.impair(clean, service, "off"))
	}()
	return work()
}
func (s *suite) delayLoss(ctx context.Context, result *caseResult) error {
	return s.withFault(ctx, "chat-service", "delay-loss", func() error {
		mark := s.sockets[1].mark()
		var ids []string
		for i := range 3 {
			id, err := s.send(ctx, 0, fmt.Sprintf("delay:%s:%d", s.rep.Run, i))
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if err := s.quiet(ctx); err != nil {
			return err
		}
		m, d, o := inspectDelivery(s.sockets[1].snapshot(mark), s.chatID, ids)
		result.Details["delivery"] = deliveryResult{m, d, o}
		if m+d+o > 0 {
			return fmt.Errorf("impaired delivery: missing=%d duplicates=%d out_of_order=%d", m, d, o)
		}
		return nil
	})
}

func (s *suite) cancellation(ctx context.Context, _ *caseResult) error {
	return s.withFault(ctx, "auth-service", "blackhole", func() error {
		for _, deadline := range []bool{true, false} {
			var call context.Context
			var cancel context.CancelFunc
			var timer *time.Timer
			if deadline {
				call, cancel = context.WithTimeout(ctx, 250*time.Millisecond)
			} else {
				call, cancel = context.WithCancel(ctx)
				timer = time.AfterFunc(100*time.Millisecond, cancel)
			}
			_, err := s.createChat(call, false)
			expected := context.Canceled
			if deadline {
				expected = context.DeadlineExceeded
			}
			cancel()
			if timer != nil {
				timer.Stop()
			}
			if !errors.Is(err, expected) {
				return fmt.Errorf("request cancellation: got %v, want %v", err, expected)
			}
		}
		return nil
	})
}

// A write with an uncertain outcome is sent ONCE, then resolved through history.
func (s *suite) uncertain(ctx context.Context, label string, action func() error, result *caseResult) error {
	text := "uncertain:" + s.rep.Run + ":" + label
	if s.uncertainWrites == nil {
		s.uncertainWrites = map[string]string{}
	}
	s.uncertainWrites[text] = ""
	marks := [2]int{s.sockets[0].mark(), s.sockets[1].mark()}
	from := s.sockets[0].mark()
	if err := s.sockets[0].send(ctx, map[string]any{"type": "send_message", "chat_id": s.chatID, "text": text}); err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	answer, err := s.sockets[0].wait(ctx, from, func(e event) bool { return e.Type == "message_sent" || e.Type == "error" })
	if err != nil {
		return err
	}
	result.Details["response"] = answer.Type
	if answer.Type == "message_sent" {
		s.uncertainWrites[text] = answer.ID
	}
	if answer.Type == "message_sent" && answer.ID == "" {
		return fmt.Errorf("ACK missing ID")
	}
	if err = s.ready(ctx); err != nil {
		return err
	}
	// Allow an already executing database transaction and relay to settle.
	if err = s.quiet(ctx); err != nil {
		return err
	}
	history, err := s.history(ctx, 1)
	if err != nil {
		return err
	}
	var ids []string
	for _, m := range history {
		if m.Text == text {
			ids = append(ids, m.ID)
		}
	}
	result.Details["stored_copies"] = len(ids)
	if len(ids) > 1 {
		return fmt.Errorf("single write created %d stored messages", len(ids))
	}
	if answer.Type == "message_sent" && (len(ids) != 1 || ids[0] != answer.ID) {
		return fmt.Errorf("successful ACK not found in history")
	}
	for i := 0; i < 2; i++ {
		m, d, o := inspectDelivery(s.sockets[i].snapshot(marks[i]), s.chatID, ids)
		result.Details[fmt.Sprintf("fault_gateway_%d", i+1)] = deliveryResult{m, d, o}
	}
	// Read-only probes wait for existing RPC clients to reconnect before a
	// new non-idempotent write. No send_message is replayed during recovery.
	if err = s.gatewayHistoryReady(ctx, result); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if _, err = s.send(ctx, i, "recovered:"+label+fmt.Sprint(i)); err != nil {
			return fmt.Errorf("gateway %d did not recover: %w", i+1, err)
		}
	}
	return nil
}

func (s *suite) blackhole(ctx context.Context, result *caseResult) error {
	if err := s.control.impair(ctx, "chat-service", "blackhole"); err != nil {
		return err
	}
	restored := false
	defer func() {
		if !restored {
			clean, cancel := cleanupContext()
			defer cancel()
			_ = s.control.impair(clean, "chat-service", "off")
		}
	}()
	return s.uncertain(ctx, "blackhole", func() error {
		// The gateway's RPC deadline is 10 seconds. Keep responses blackholed
		// beyond it, then restore the path before resolving the write in history.
		timer := time.NewTimer(11 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		if err := s.control.impair(ctx, "chat-service", "off"); err != nil {
			return err
		}
		restored = true
		return nil
	}, result)
}

func (s *suite) restartCase(service string, crash bool) func(context.Context, *caseResult) error {
	return func(ctx context.Context, result *caseResult) error {
		label := service + "-graceful"
		if crash {
			label = service + "-crash"
		}
		if service == "chat-service" {
			return s.uncertain(ctx, label, func() error { return s.restartService(ctx, service, crash) }, result)
		}
		if err := s.restartService(ctx, service, crash); err != nil {
			return err
		}
		if err := s.ready(ctx); err != nil {
			return err
		}
		// Always traverse chat -> auth, even when the private chat already exists.
		attempts := 0
		// CreateChat for this already existing private pair is idempotent:
		// it traverses the reused chat -> auth client and returns the same ID.
		err := poll(ctx, func() error {
			attempts++
			id, err := s.createChat(ctx, false)
			if err != nil {
				return err
			}
			if id != s.chatID {
				return fmt.Errorf("existing chat changed after auth restart")
			}
			return nil
		})
		result.Details["existing_chat_readiness_attempts"] = attempts
		if err != nil {
			return err
		}
		_, err = s.send(ctx, 1, "after:"+label)
		return err
	}
}

func (s *suite) gatewayHistoryReady(ctx context.Context, result *caseResult) error {
	for i := 0; i < 2; i++ {
		attempts := 0
		err := poll(ctx, func() error {
			attempts++
			sock := s.sockets[i]
			from := sock.mark()
			if err := sock.send(ctx, map[string]any{"type": "get_history", "chat_id": s.chatID, "limit": 100}); err != nil {
				return err
			}
			answer, err := sock.wait(ctx, from, func(e event) bool { return e.Type == "history" || e.Type == "error" })
			if err != nil {
				return err
			}
			if answer.Type == "error" {
				return fmt.Errorf("history readiness: %s", answer.Error)
			}
			if answer.Chat != s.chatID {
				return fmt.Errorf("history response belongs to another chat")
			}
			return nil
		})
		result.Details[fmt.Sprintf("gateway_%d_readiness_attempts", i+1)] = attempts
		if err != nil {
			return err
		}
	}
	return nil
}

// Docker can allocate another published host port when a stopped container
// starts again. Interservice DNS addresses stay internal to the test network.
func (s *suite) restartService(ctx context.Context, service string, crash bool) error {
	if err := s.control.restart(ctx, service, crash); err != nil {
		return err
	}
	endpoint, err := s.control.endpoint(ctx, service, "8080")
	if err != nil {
		return err
	}
	if service == "auth-service" {
		s.auth = endpoint
	} else {
		s.chat = endpoint
	}
	return nil
}
