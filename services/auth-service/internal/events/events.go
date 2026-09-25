package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	StreamName = "USER_EVENTS"

	SubjectUserRegistered     = "user.registered"
	SubjectUserPasswordReset  = "user.password_reset"
	SubjectUserOAuthLinked    = "user.oauth_linked"
	SubjectUserProfileUpdated = "user.profile_updated"
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
		Subjects: []string{"user.*"},
	})
	if err != nil {
		return nil, err
	}

	return &Publisher{js: js}, nil
}

func (p *Publisher) PublishUserRegistered(ctx context.Context, event domain.UserRegistered) error {
	return p.publish(ctx, SubjectUserRegistered, event)
}

func (p *Publisher) PublishUserPasswordReset(ctx context.Context, event domain.UserPasswordReset) error {
	return p.publish(ctx, SubjectUserPasswordReset, event)
}

func (p *Publisher) PublishUserOAuthLinked(ctx context.Context, event domain.UserOAuthLinked) error {
	return p.publish(ctx, SubjectUserOAuthLinked, event)
}

func (p *Publisher) PublishUserProfileUpdated(ctx context.Context, event domain.UserProfileUpdated) error {
	return p.publish(ctx, SubjectUserProfileUpdated, event)
}

func (p *Publisher) publish(ctx context.Context, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	ctx, header, span := tracing.StartPublishSpan(ctx, subject)
	defer span.End()

	_, err = p.js.PublishMsg(ctx, &nats.Msg{Subject: subject, Data: payload, Header: header})
	return err
}
