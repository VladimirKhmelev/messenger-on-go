package events

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

// fakeStore mimics RelayOutbox: publishes pending messages in order and
// keeps the failed one and everything after it.
type fakeStore struct {
	mu      sync.Mutex
	pending []domain.OutboxMessage
}

func (s *fakeStore) RelayOutbox(ctx context.Context, limit int, publish func(context.Context, domain.OutboxMessage) error) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for len(s.pending) > 0 && n < limit {
		if err := publish(ctx, s.pending[0]); err != nil {
			return n, err
		}
		s.pending = s.pending[1:]
		n++
	}
	return n, nil
}

func (s *fakeStore) left() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

type fakeJS struct {
	mu     sync.Mutex
	sent   []*nats.Msg
	failAt int
	calls  int
}

func (j *fakeJS) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.calls++
	if j.failAt == j.calls {
		return nil, errors.New("nats down")
	}
	j.sent = append(j.sent, msg)
	return &jetstream.PubAck{}, nil
}

func (j *fakeJS) count() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.sent)
}

func messages(n int) []domain.OutboxMessage {
	out := make([]domain.OutboxMessage, n)
	for i := range out {
		out[i] = domain.OutboxMessage{
			ID: int64(i + 1), Subject: SubjectMessageCreated, Payload: []byte(`{}`),
			Headers: map[string][]string{"Traceparent": {"00-trace-span-01"}},
		}
	}
	return out
}

func TestRelay_PublishesInOrderWithDedupIDAndTraceHeaders(t *testing.T) {
	store := &fakeStore{pending: messages(3)}
	js := &fakeJS{}
	NewRelay(js, store).drain(context.Background())

	if store.left() != 0 || js.count() != 3 {
		t.Fatalf("left %d, sent %d; want 0 and 3", store.left(), js.count())
	}
	for i, msg := range js.sent {
		if want := "chat-outbox-" + strconv.Itoa(i+1); msg.Header.Get(jetstream.MsgIDHeader) != want {
			t.Errorf("msg %d Nats-Msg-Id = %q, want %q", i, msg.Header.Get(jetstream.MsgIDHeader), want)
		}
		if msg.Header.Get("Traceparent") != "00-trace-span-01" {
			t.Errorf("msg %d lost trace header: %v", i, msg.Header)
		}
	}
}

func TestRelay_DrainsMoreThanOneBatch(t *testing.T) {
	store := &fakeStore{pending: messages(relayBatchSize*2 + 5)}
	js := &fakeJS{}
	NewRelay(js, store).drain(context.Background())

	if store.left() != 0 {
		t.Errorf("left %d after drain, want 0", store.left())
	}
}

func TestRelay_KeepsUnpublishedOnError(t *testing.T) {
	store := &fakeStore{pending: messages(5)}
	js := &fakeJS{failAt: 3}
	NewRelay(js, store).drain(context.Background())

	if js.count() != 2 || store.left() != 3 {
		t.Fatalf("sent %d, left %d; want 2 sent and 3 kept for retry", js.count(), store.left())
	}
	if store.pending[0].ID != 3 {
		t.Errorf("next pending ID = %d, want 3 (order kept)", store.pending[0].ID)
	}
}

func TestRelay_WakePublishesWithoutWaitingForPoll(t *testing.T) {
	store := &fakeStore{}
	js := &fakeJS{}
	relay := NewRelay(js, store)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); relay.Run(ctx) }()
	defer func() { cancel(); <-done }()

	time.Sleep(20 * time.Millisecond) // first (empty) drain done, Run is waiting
	store.mu.Lock()
	store.pending = messages(1)
	store.mu.Unlock()
	relay.Wake()

	deadline := time.Now().Add(relayPollInterval / 2)
	for js.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if js.count() != 1 {
		t.Fatalf("event not published within %v of Wake, poll interval is %v", relayPollInterval/2, relayPollInterval)
	}
}

func TestRelay_WakeNeverBlocks(t *testing.T) {
	relay := NewRelay(&fakeJS{}, &fakeStore{})
	for i := 0; i < 10; i++ {
		relay.Wake() // nobody is running; must not block
	}
}
