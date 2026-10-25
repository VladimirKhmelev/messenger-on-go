package events

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
)

const pullBatch = 10

var (
	pullMaxWait = 5 * time.Second
	// retryPause is the wait after a failed fetch.
	retryPause = time.Second
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

// consumeOne hands the messages of a durable consumer to sub.Handle until
// ctx is done. A failed fetch returns at once, so the next one waits for
// retryPause; the consumer is not created anew, since a new durable
// consumer would deliver the whole stream again.
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
			sleep(ctx, retryPause)
			continue
		}

		for msg := range msgs.Messages() {
			msgCtx, span := tracing.StartConsumeSpan(ctx, msg.Subject(), msg.Headers())
			sub.Handle(msgCtx, msg.Subject(), msg.Data())
			span.End()
			if err := msg.Ack(); err != nil {
				log.Printf("notification-worker: failed to ack %s message: %v", sub.FilterSubject, err)
			}
		}
		if err := msgs.Error(); err != nil {
			log.Printf("notification-worker: failed to fetch %s batch: %v", sub.FilterSubject, err)
			sleep(ctx, retryPause)
		}
	}
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
