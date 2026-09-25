package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
)

const (
	NotifyStreamName = "NOTIFY_EVENTS"

	SubjectNotifyPush = "notify.push"
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
		Name:     NotifyStreamName,
		Subjects: []string{"notify.*"},
	})
	if err != nil {
		return nil, err
	}

	return &Publisher{js: js}, nil
}

func (p *Publisher) PublishNotifyPush(ctx context.Context, event NotifyPush) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	ctx, header, span := tracing.StartPublishSpan(ctx, SubjectNotifyPush)
	defer span.End()

	_, err = p.js.PublishMsg(ctx, &nats.Msg{Subject: SubjectNotifyPush, Data: payload, Header: header})
	if err != nil {
		return err
	}
	metrics.NotifyPushTotal.Inc()
	return nil
}
