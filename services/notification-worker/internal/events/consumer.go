package events

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	pullMaxWait = 5 * time.Second
	pullBatch   = 10
)

type Subscription struct {
	StreamName    string
	ConsumerName  string
	FilterSubject string
	Handle        func(ctx context.Context, subject string, data []byte)
}

func Consume(ctx context.Context, url string, subs []Subscription) error {
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}

	errCh := make(chan error, len(subs))
	for _, sub := range subs {
		go func(sub Subscription) {
			errCh <- consumeOne(ctx, js, sub)
		}(sub)
	}

	for range subs {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func consumeOne(ctx context.Context, js jetstream.JetStream, sub Subscription) error {
	stream, err := js.Stream(ctx, sub.StreamName)
	if err != nil {
		return err
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       sub.ConsumerName,
		FilterSubject: sub.FilterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
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
			log.Printf("notification-worker: failed to fetch %s batch: %v", sub.FilterSubject, err)
			continue
		}

		for msg := range msgs.Messages() {
			sub.Handle(ctx, msg.Subject(), msg.Data())
			if err := msg.Ack(); err != nil {
				log.Printf("notification-worker: failed to ack %s message: %v", sub.FilterSubject, err)
			}
		}
	}
}
