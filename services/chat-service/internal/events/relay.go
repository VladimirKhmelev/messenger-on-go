package events

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

const (
	relayBatchSize    = 100
	relayPollInterval = time.Second
)

type RelayStore interface {
	RelayOutbox(ctx context.Context, limit int, publish func(ctx context.Context, msg domain.OutboxMessage) error) (int, error)
}

type msgPublisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Relay moves committed outbox events to JetStream. Delivery is
// at-least-once on our side (an event is deleted only after the broker
// acknowledged it); the outbox ID goes out as Nats-Msg-Id, so JetStream
// drops a resend of the same event within its dedup window.
type Relay struct {
	js    msgPublisher
	store RelayStore
	wake  chan struct{}
}

func NewRelay(js msgPublisher, store RelayStore) *Relay {
	return &Relay{js: js, store: store, wake: make(chan struct{}, 1)}
}

// Wake asks the relay to publish now. It never blocks: if a wake-up is
// already pending, that one will pick up the new events too.
func (r *Relay) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run publishes until ctx is canceled.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(relayPollInterval)
	defer ticker.Stop()

	for {
		r.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-ticker.C:
		}
	}
}

func (r *Relay) drain(ctx context.Context) {
	for ctx.Err() == nil {
		n, err := r.store.RelayOutbox(ctx, relayBatchSize, r.publish)
		if err != nil {
			log.Printf("chat-service: outbox relay: %v", err)
			return
		}
		if n < relayBatchSize {
			return
		}
	}
}

func (r *Relay) publish(ctx context.Context, msg domain.OutboxMessage) error {
	header := nats.Header{}
	for k, v := range msg.Headers {
		header[k] = v
	}
	header.Set(jetstream.MsgIDHeader, "chat-outbox-"+strconv.FormatInt(msg.ID, 10))
	_, err := r.js.PublishMsg(ctx, &nats.Msg{Subject: msg.Subject, Data: msg.Payload, Header: header})
	return err
}
