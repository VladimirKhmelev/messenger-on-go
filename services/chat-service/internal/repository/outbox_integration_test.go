//go:build integration

package repository

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

// The outbox table is shared by every test in the package: start each test
// from an empty one and restore the commit hook afterwards.
func resetOutbox(t *testing.T) *atomic.Int32 {
	t.Helper()
	if _, err := testRepo.conn.Exec(`DELETE FROM outbox`); err != nil {
		t.Fatalf("clean outbox: %v", err)
	}
	var wakes atomic.Int32
	testRepo.OnCommit(func() { wakes.Add(1) })
	t.Cleanup(func() { testRepo.OnCommit(nil) })
	return &wakes
}

func relayAll(t *testing.T) []domain.OutboxMessage {
	t.Helper()
	var got []domain.OutboxMessage
	if _, err := testRepo.RelayOutbox(context.Background(), 100, func(_ context.Context, m domain.OutboxMessage) error {
		got = append(got, m)
		return nil
	}); err != nil {
		t.Fatalf("RelayOutbox() error: %v", err)
	}
	return got
}

func enqueue(ctx context.Context, subject string) error {
	return testRepo.EnqueueOutbox(ctx, subject, []byte(`{"s":"`+subject+`"}`), map[string][]string{"traceparent": {"00-abc-def-01"}})
}

func TestOutbox_EnqueueOutsideTransactionIsRejected(t *testing.T) {
	resetOutbox(t)

	if err := enqueue(context.Background(), "msg.created"); !errors.Is(err, errOutboxOutsideTx) {
		t.Fatalf("EnqueueOutbox() outside tx error = %v, want %v", err, errOutboxOutsideTx)
	}
	if got := relayAll(t); len(got) != 0 {
		t.Errorf("outbox has %d events, want none", len(got))
	}
}

// The whole point of the outbox: the event and the change it describes
// commit or roll back together.
func TestOutbox_RollbackDropsBothMessageAndEvent(t *testing.T) {
	wakes := resetOutbox(t)
	chat := createPrivateChat(t, now(), uuid.NewString(), uuid.NewString())
	msg := &domain.Message{ID: uuid.NewString(), ChatID: chat.ID, SenderID: uuid.NewString(), Body: "hi", CreatedAt: now()}
	boom := errors.New("boom")

	err := testRepo.WithTx(context.Background(), func(ctx context.Context) error {
		if err := testRepo.CreateMessage(ctx, msg); err != nil {
			return err
		}
		if err := enqueue(ctx, "msg.created"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithTx() error = %v, want %v", err, boom)
	}

	if _, err := testRepo.GetMessage(context.Background(), msg.ID); !errors.Is(err, domain.ErrMessageNotFound) {
		t.Errorf("message after rollback: err = %v, want %v", err, domain.ErrMessageNotFound)
	}
	if got := relayAll(t); len(got) != 0 {
		t.Errorf("outbox after rollback has %d events, want none", len(got))
	}
	if wakes.Load() != 0 {
		t.Errorf("relay woken %d times after rollback, want 0", wakes.Load())
	}
}

func TestOutbox_CommitStoresEventsInOrderAndWakesRelayOnce(t *testing.T) {
	wakes := resetOutbox(t)
	chat := createPrivateChat(t, now(), uuid.NewString(), uuid.NewString())
	msg := &domain.Message{ID: uuid.NewString(), ChatID: chat.ID, SenderID: uuid.NewString(), Body: "hi", CreatedAt: now()}

	err := testRepo.WithTx(context.Background(), func(ctx context.Context) error {
		if err := testRepo.CreateMessage(ctx, msg); err != nil {
			return err
		}
		for _, s := range []string{"msg.created", "msg.read", "chat.deleted"} {
			if err := enqueue(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx() error: %v", err)
	}
	if wakes.Load() != 1 {
		t.Errorf("relay woken %d times, want 1 per commit", wakes.Load())
	}

	got := relayAll(t)
	if len(got) != 3 {
		t.Fatalf("relayed %d events, want 3", len(got))
	}
	for i, want := range []string{"msg.created", "msg.read", "chat.deleted"} {
		if got[i].Subject != want || string(got[i].Payload) != `{"s":"`+want+`"}` {
			t.Errorf("event %d = %s %s, want %s", i, got[i].Subject, got[i].Payload, want)
		}
		if got[i].Headers["traceparent"][0] != "00-abc-def-01" {
			t.Errorf("event %d lost trace header: %v", i, got[i].Headers)
		}
	}
	if again := relayAll(t); len(again) != 0 {
		t.Errorf("second relay returned %d events, want 0: published events must be deleted", len(again))
	}
	// the relay's own commits carry no new events and must not wake it,
	// otherwise it would spin on an empty outbox
	if wakes.Load() != 1 {
		t.Errorf("relay woken %d times after relaying, want still 1", wakes.Load())
	}
}

func TestOutbox_PublishErrorKeepsFailedAndLaterEvents(t *testing.T) {
	resetOutbox(t)
	_ = testRepo.WithTx(context.Background(), func(ctx context.Context) error {
		for _, s := range []string{"e1", "e2", "e3"} {
			if err := enqueue(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})

	natsDown := errors.New("nats down")
	n, err := testRepo.RelayOutbox(context.Background(), 100, func(_ context.Context, m domain.OutboxMessage) error {
		if m.Subject == "e2" {
			return natsDown
		}
		return nil
	})
	if n != 1 || !errors.Is(err, natsDown) {
		t.Fatalf("RelayOutbox() = %d, %v; want 1 published and %v", n, err, natsDown)
	}

	var subjects []string
	for _, m := range relayAll(t) {
		subjects = append(subjects, m.Subject)
	}
	if len(subjects) != 2 || subjects[0] != "e2" || subjects[1] != "e3" {
		t.Errorf("retry relayed %v, want [e2 e3]: e1 must stay deleted, order kept", subjects)
	}
}

// Two relays (e.g. two chat-service instances) must never publish the same
// event: the second skips rows the first has locked.
func TestOutbox_ConcurrentRelaysSkipLockedEvents(t *testing.T) {
	resetOutbox(t)
	_ = testRepo.WithTx(context.Background(), func(ctx context.Context) error {
		for _, s := range []string{"e1", "e2", "e3"} {
			if err := enqueue(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})

	holding := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan []string)
	go func() {
		var got []string
		_, _ = testRepo.RelayOutbox(context.Background(), 1, func(_ context.Context, m domain.OutboxMessage) error {
			got = append(got, m.Subject)
			close(holding)
			<-release
			return nil
		})
		firstDone <- got
	}()

	<-holding // first relay holds the lock on e1
	var second []string
	if _, err := testRepo.RelayOutbox(context.Background(), 100, func(_ context.Context, m domain.OutboxMessage) error {
		second = append(second, m.Subject)
		return nil
	}); err != nil {
		t.Fatalf("second RelayOutbox() error: %v", err)
	}
	close(release)

	select {
	case first := <-firstDone:
		if len(first) != 1 || first[0] != "e1" {
			t.Errorf("first relay got %v, want [e1]", first)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first relay did not finish")
	}
	if len(second) != 2 || second[0] != "e2" || second[1] != "e3" {
		t.Errorf("second relay got %v, want [e2 e3] (e1 was locked by the first)", second)
	}
}

func TestOutbox_NestedWithTxJoinsOuterTransaction(t *testing.T) {
	resetOutbox(t)
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: now(), ChatType: domain.ChatTypePrivate}

	// CreateChat opens its own WithTx; inside an outer one it must join it,
	// so the outer rollback undoes the chat as well
	_ = testRepo.WithTx(context.Background(), func(ctx context.Context) error {
		if err := testRepo.CreateChat(ctx, chat, map[string]domain.MemberChatKey{alice: key(alice), bob: key(bob)}); err != nil {
			return err
		}
		return errors.New("abort")
	})

	if _, err := testRepo.GetChat(context.Background(), chat.ID); !errors.Is(err, domain.ErrChatNotFound) {
		t.Errorf("GetChat() after outer rollback: err = %v, want %v", err, domain.ErrChatNotFound)
	}
}
