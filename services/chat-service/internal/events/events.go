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

type Publisher struct {
	js jetstream.JetStream
}

func Connect(ctx context.Context, url string) (*Publisher, error) {
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

	return &Publisher{js: js}, nil
}

func (p *Publisher) PublishMessageCreated(ctx context.Context, event domain.MessageCreated) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.publish(ctx, SubjectMessageCreated, payload)
}

func (p *Publisher) PublishMessageUpdated(ctx context.Context, event domain.MessageUpdated) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	subject := SubjectMessageUpdated
	if event.Deleted {
		subject = SubjectMessageDeleted
	}
	return p.publish(ctx, subject, payload)
}

func (p *Publisher) PublishMessageRead(ctx context.Context, event domain.MessageRead) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.publish(ctx, SubjectMessageRead, payload)
}

func (p *Publisher) PublishChatDeleted(ctx context.Context, event domain.ChatDeleted) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.publish(ctx, SubjectChatDeleted, payload)
}

func (p *Publisher) publish(ctx context.Context, subject string, payload []byte) error {
	ctx, header, span := tracing.StartPublishSpan(ctx, subject)
	defer span.End()

	_, err := p.js.PublishMsg(ctx, &nats.Msg{Subject: subject, Data: payload, Header: header})
	return err
}
