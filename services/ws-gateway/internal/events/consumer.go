package events

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	chatStreamName = "CHAT_EVENTS"
	subjectMsg     = "msg.created"

	notifyStreamName = "NOTIFY_EVENTS"
	subjectNotify    = "notify.push"

	pullMaxWait = 5 * time.Second
	pullBatch   = 10
)

type MessageCreated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
}

type NotifyPush struct {
	UserID    string    `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Handlers struct {
	OnMessageCreated func(ctx context.Context, event MessageCreated)
	OnNotifyPush     func(ctx context.Context, event NotifyPush)
}

func Consume(ctx context.Context, url string, handlers Handlers) error {
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)

	go func() {
		errCh <- consumeOne(ctx, js, chatStreamName, subjectMsg, func(ctx context.Context, data []byte) {
			var event MessageCreated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.created event: %v", err)
				return
			}
			handlers.OnMessageCreated(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, notifyStreamName, subjectNotify, func(ctx context.Context, data []byte) {
			var event NotifyPush
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal notify.push event: %v", err)
				return
			}
			handlers.OnNotifyPush(ctx, event)
		})
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func consumeOne(ctx context.Context, js jetstream.JetStream, streamName, filterSubject string, handle func(ctx context.Context, data []byte)) error {
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return err
	}

	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckNonePolicy,
	})
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := consumer.Fetch(pullBatch, jetstream.FetchMaxWait(pullMaxWait))
		if err != nil {
			log.Printf("ws-gateway: failed to fetch %s batch: %v", filterSubject, err)
			continue
		}

		for msg := range msgs.Messages() {
			handle(ctx, msg.Data())
		}
	}
}
