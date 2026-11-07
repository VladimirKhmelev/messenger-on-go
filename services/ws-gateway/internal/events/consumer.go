package events

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

const (
	chatStreamName    = "CHAT_EVENTS"
	subjectMsg        = "msg.created"
	subjectMsgEdit    = "msg.updated"
	subjectMsgDelete  = "msg.deleted"
	subjectMsgRead    = "msg.read"
	subjectChatDelete = "chat.deleted"

	notifyStreamName = "NOTIFY_EVENTS"
	subjectNotify    = "notify.push"

	userStreamName        = "USER_EVENTS"
	subjectProfileUpdated = "user.profile_updated"

	subjectPresence = "user.presence"
	subjectTyping   = "chat.typing"

	pullBatch = 10

	// inactiveThreshold is how long NATS keeps a consumer that fetches
	// nothing, far above the time a batch takes to handle.
	inactiveThreshold = time.Minute
)

var (
	pullMaxWait = 5 * time.Second
	// retryPause is the wait before a failed consumer is created anew.
	retryPause = time.Second
)

type Handlers struct {
	OnMessageCreated  func(ctx context.Context, event domain.MessageCreated) error
	OnMessageUpdated  func(ctx context.Context, event domain.MessageUpdated) error
	OnMessageRead     func(ctx context.Context, event domain.MessageRead) error
	OnNotifyPush      func(ctx context.Context, event domain.NotifyPush) error
	OnPresenceChanged func(ctx context.Context, event domain.PresenceChanged) error
	OnProfileUpdated  func(ctx context.Context, event domain.ProfileUpdated) error
	OnTypingChanged   func(ctx context.Context, event domain.TypingChanged) error
	OnChatDeleted     func(ctx context.Context, event domain.ChatDeleted) error
}

type PresencePublisher struct {
	nc *nats.Conn
}

func ConnectPresencePublisher(url string) (*PresencePublisher, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	return &PresencePublisher{nc: nc}, nil
}

func (p *PresencePublisher) Close() {
	p.nc.Close()
}

func (p *PresencePublisher) PublishPresenceChanged(ctx context.Context, event domain.PresenceChanged) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, header, span := tracing.StartPublishSpan(ctx, subjectPresence)
	defer span.End()

	return p.nc.PublishMsg(&nats.Msg{Subject: subjectPresence, Data: payload, Header: header})
}

func (p *PresencePublisher) PublishTypingChanged(ctx context.Context, event domain.TypingChanged) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, header, span := tracing.StartPublishSpan(ctx, subjectTyping)
	defer span.End()

	return p.nc.PublishMsg(&nats.Msg{Subject: subjectTyping, Data: payload, Header: header})
}

// Consume uses the default fanout limits.
func Consume(ctx context.Context, url string, handlers Handlers) error {
	return ConsumeWithConfig(ctx, url, handlers, DefaultConfig())
}

func ConsumeWithConfig(ctx context.Context, url string, handlers Handlers, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	p := newPool(ctx, cfg)
	defer p.close()
	// A full queue may hold a batch until each bounded job has freed a slot.
	// Keep the ephemeral consumer alive through that interval.
	lifetime := max(inactiveThreshold, time.Duration(pullBatch+2)*cfg.Timeout)
	dispatch := func(subject string, header nats.Header, data []byte) error {
		key, handle, err := decodeEvent(subject, data, handlers)
		if err != nil {
			log.Printf("ws-gateway: invalid %s event: %v", subject, err)
			return nil
		}
		return p.submit(key, func(ctx context.Context) error {
			ctx, span := tracing.StartConsumeSpan(ctx, subject, header)
			defer span.End()
			return handle(ctx)
		})
	}
	core := func(subject string) func() error {
		return func() error {
			sub, err := nc.Subscribe(subject, func(msg *nats.Msg) { _ = dispatch(msg.Subject, msg.Header, msg.Data) })
			if err != nil {
				return err
			}
			defer func() { _ = sub.Unsubscribe() }()
			<-ctx.Done()
			return nil
		}
	}
	stream := func(name string, subjects []string) func() error {
		return func() error {
			return consumeMessages(ctx, js, name, subjects, lifetime, func(_ context.Context, msg jetstream.Msg) error {
				return dispatch(msg.Subject(), msg.Headers(), msg.Data())
			})
		}
	}
	readers := []func() error{
		stream(chatStreamName, []string{subjectMsg, subjectMsgEdit, subjectMsgDelete, subjectMsgRead, subjectChatDelete}),
		stream(notifyStreamName, []string{subjectNotify}),
		stream(userStreamName, []string{subjectProfileUpdated}),
		core(subjectPresence), core(subjectTyping),
	}
	errors := make(chan error, len(readers))
	var wg sync.WaitGroup
	for _, read := range readers {
		wg.Go(func() { errors <- read() })
	}
	var first error
	for range readers {
		if err := <-errors; err != nil && first == nil {
			first = err
			cancel()
		}
	}
	wg.Wait()
	return first
}

func consumeOne(ctx context.Context, js jetstream.JetStream, streamName, filterSubject string, handle func(ctx context.Context, data []byte)) error {
	return consumeMulti(ctx, js, streamName, []string{filterSubject}, handle)
}

// consumeMulti hands the messages on the subjects to handle until ctx is
// done. The consumer delivers only the messages published after it is
// created: the sessions it fans out to get the older ones from the history
// of their chats. A consumer that fails, such as one NATS has deleted, is
// created anew after a pause; the messages meanwhile are not delivered.
func consumeMulti(ctx context.Context, js jetstream.JetStream, streamName string, filterSubjects []string, handle func(ctx context.Context, data []byte)) error {
	return consumeMessages(ctx, js, streamName, filterSubjects, inactiveThreshold, func(ctx context.Context, msg jetstream.Msg) error {
		msgCtx, span := tracing.StartConsumeSpan(ctx, msg.Subject(), msg.Headers())
		defer span.End()
		handle(msgCtx, msg.Data())
		return nil
	})
}

func consumeMessages(ctx context.Context, js jetstream.JetStream, streamName string, filterSubjects []string, lifetime time.Duration, handle func(context.Context, jetstream.Msg) error) error {
	consumer, err := newConsumer(ctx, js, streamName, filterSubjects, lifetime)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	for ctx.Err() == nil {
		if consumer == nil {
			if !sleep(ctx, retryPause) {
				return nil
			}
			c, err := newConsumer(ctx, js, streamName, filterSubjects, lifetime)
			if err != nil {
				log.Printf("ws-gateway: failed to create %v consumer: %v", filterSubjects, err)
				continue
			}
			log.Printf("ws-gateway: created %v consumer anew", filterSubjects)
			consumer = c
		}

		msgs, err := consumer.Fetch(pullBatch, jetstream.FetchMaxWait(pullMaxWait))
		if err != nil {
			log.Printf("ws-gateway: failed to fetch %v batch: %v", filterSubjects, err)
			consumer = nil
			continue
		}

		for msg := range msgs.Messages() {
			if err := handle(ctx, msg); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
		if err := msgs.Error(); err != nil {
			log.Printf("ws-gateway: failed to fetch %v batch: %v", filterSubjects, err)
			consumer = nil
		}
	}
	return nil
}

func newConsumer(ctx context.Context, js jetstream.JetStream, streamName string, filterSubjects []string, lifetime time.Duration) (jetstream.Consumer, error) {
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}
	return stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		AckPolicy:         jetstream.AckNonePolicy,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		InactiveThreshold: lifetime,
	})
}

// sleep waits for d and reports whether ctx is still live.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
