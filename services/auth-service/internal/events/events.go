package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	StreamName = "USER_EVENTS"

	SubjectUserRegistered    = "user.registered"
	SubjectUserPasswordReset = "user.password_reset"
	SubjectUserOAuthLinked   = "user.oauth_linked"
)

type UserRegistered struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"created_at"`
}

type UserPasswordReset struct {
	UserID string    `json:"user_id"`
	Email  string    `json:"email"`
	At     time.Time `json:"at"`
}

type UserOAuthLinked struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Provider string    `json:"provider"`
	At       time.Time `json:"at"`
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
		Subjects: []string{"user.*"},
	})
	if err != nil {
		return nil, err
	}

	return &Publisher{js: js}, nil
}

func (p *Publisher) PublishUserRegistered(ctx context.Context, event UserRegistered) error {
	return p.publish(ctx, SubjectUserRegistered, event)
}

func (p *Publisher) PublishUserPasswordReset(ctx context.Context, event UserPasswordReset) error {
	return p.publish(ctx, SubjectUserPasswordReset, event)
}

func (p *Publisher) PublishUserOAuthLinked(ctx context.Context, event UserOAuthLinked) error {
	return p.publish(ctx, SubjectUserOAuthLinked, event)
}

func (p *Publisher) publish(ctx context.Context, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = p.js.Publish(ctx, subject, payload)
	return err
}
