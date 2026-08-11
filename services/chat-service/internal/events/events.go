package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	StreamName = "CHAT_EVENTS"

	SubjectMessageCreated = "msg.created"
	SubjectMessageUpdated = "msg.updated"
	SubjectMessageDeleted = "msg.deleted"
)

type MessageCreated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageUpdated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	NewBody   *string   `json:"new_body,omitempty"`
	Deleted   bool      `json:"deleted"`
	UpdatedAt time.Time `json:"updated_at"`
}

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
		Subjects: []string{"msg.*"},
	})
	if err != nil {
		return nil, err
	}

	return &Publisher{js: js}, nil
}

func (p *Publisher) PublishMessageCreated(ctx context.Context, event MessageCreated) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = p.js.Publish(ctx, SubjectMessageCreated, payload)
	return err
}

func (p *Publisher) PublishMessageUpdated(ctx context.Context, event MessageUpdated) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	subject := SubjectMessageUpdated
	if event.Deleted {
		subject = SubjectMessageDeleted
	}

	_, err = p.js.Publish(ctx, subject, payload)
	return err
}
