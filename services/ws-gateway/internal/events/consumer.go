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
	streamName  = "CHAT_EVENTS"
	subjectMsg  = "msg.created"
	pullMaxWait = 5 * time.Second
	pullBatch   = 10
)

type MessageCreated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Handler func(ctx context.Context, event MessageCreated)

func Consume(ctx context.Context, url string, handler Handler) error {
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return err
	}

	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: subjectMsg,
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
			log.Printf("ws-gateway: failed to fetch msg.created batch: %v", err)
			continue
		}

		for msg := range msgs.Messages() {
			var event MessageCreated
			if err := json.Unmarshal(msg.Data(), &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.created event: %v", err)
				continue
			}
			handler(ctx, event)
		}
	}
}
