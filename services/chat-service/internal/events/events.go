package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

const (
	StreamName = "CHAT_EVENTS"

	SubjectMessageCreated = "msg.created"
	SubjectMessageUpdated = "msg.updated"
	SubjectMessageDeleted = "msg.deleted"
	SubjectMessageRead    = "msg.read"
	SubjectChatDeleted    = "chat.deleted"
)

// Connect opens JetStream and makes sure the stream the relay publishes to
// exists.
func Connect(ctx context.Context, url string) (jetstream.JetStream, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{"msg.*", "chat.*"},
	})
	if err != nil {
		return nil, err
	}
	return js, nil
}

type OutboxStore interface {
	EnqueueOutbox(ctx context.Context, subject string, payload []byte, headers map[string][]string) error
}

type OutboxPublisher struct {
	store OutboxStore
}

func NewOutboxPublisher(store OutboxStore) *OutboxPublisher {
	return &OutboxPublisher{store: store}
}

func (p *OutboxPublisher) PublishMessageCreated(ctx context.Context, event domain.MessageCreated) error {
	return p.enqueue(ctx, SubjectMessageCreated, event)
}

func (p *OutboxPublisher) PublishMessageUpdated(ctx context.Context, event domain.MessageUpdated) error {
	subject := SubjectMessageUpdated
	if event.Deleted {
		subject = SubjectMessageDeleted
	}
	return p.enqueue(ctx, subject, event)
}

func (p *OutboxPublisher) PublishMessageRead(ctx context.Context, event domain.MessageRead) error {
	return p.enqueue(ctx, SubjectMessageRead, event)
}

func (p *OutboxPublisher) PublishChatDeleted(ctx context.Context, event domain.ChatDeleted) error {
	return p.enqueue(ctx, SubjectChatDeleted, event)
}

// enqueue stores the trace context of the request that produced the event,
// so consumers stay linked to it even though NATS delivery happens later
// from the relay.
func (p *OutboxPublisher) enqueue(ctx context.Context, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ctx, header, span := tracing.StartPublishSpan(ctx, subject)
	defer span.End()
	return p.store.EnqueueOutbox(ctx, subject, payload, header)
}
