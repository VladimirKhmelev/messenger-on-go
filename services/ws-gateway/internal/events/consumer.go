package events

import (
	"context"
	"encoding/json"
	"log"
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

	pullMaxWait = 5 * time.Second
	pullBatch   = 10
)

type Handlers struct {
	OnMessageCreated  func(ctx context.Context, event domain.MessageCreated)
	OnMessageUpdated  func(ctx context.Context, event domain.MessageUpdated)
	OnMessageRead     func(ctx context.Context, event domain.MessageRead)
	OnNotifyPush      func(ctx context.Context, event domain.NotifyPush)
	OnPresenceChanged func(ctx context.Context, event domain.PresenceChanged)
	OnProfileUpdated  func(ctx context.Context, event domain.ProfileUpdated)
	OnTypingChanged   func(ctx context.Context, event domain.TypingChanged)
	OnChatDeleted     func(ctx context.Context, event domain.ChatDeleted)
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

	errCh := make(chan error, 8)

	go func() {
		errCh <- consumeOne(ctx, js, chatStreamName, subjectMsg, func(ctx context.Context, data []byte) {
			var event domain.MessageCreated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.created event: %v", err)
				return
			}
			handlers.OnMessageCreated(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeMulti(ctx, js, chatStreamName, []string{subjectMsgEdit, subjectMsgDelete}, func(ctx context.Context, data []byte) {
			var event domain.MessageUpdated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.updated/deleted event: %v", err)
				return
			}
			handlers.OnMessageUpdated(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, chatStreamName, subjectMsgRead, func(ctx context.Context, data []byte) {
			var event domain.MessageRead
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.read event: %v", err)
				return
			}
			handlers.OnMessageRead(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, chatStreamName, subjectChatDelete, func(ctx context.Context, data []byte) {
			var event domain.ChatDeleted
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal chat.deleted event: %v", err)
				return
			}
			handlers.OnChatDeleted(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, notifyStreamName, subjectNotify, func(ctx context.Context, data []byte) {
			var event domain.NotifyPush
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal notify.push event: %v", err)
				return
			}
			handlers.OnNotifyPush(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, userStreamName, subjectProfileUpdated, func(ctx context.Context, data []byte) {
			var event domain.ProfileUpdated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal user.profile_updated event: %v", err)
				return
			}
			handlers.OnProfileUpdated(ctx, event)
		})
	}()

	go func() {
		sub, err := nc.Subscribe(subjectPresence, func(msg *nats.Msg) {
			var event domain.PresenceChanged
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal user.presence event: %v", err)
				return
			}
			msgCtx, span := tracing.StartConsumeSpan(ctx, subjectPresence, msg.Header)
			handlers.OnPresenceChanged(msgCtx, event)
			span.End()
		})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = sub.Unsubscribe() }()

		<-ctx.Done()
		errCh <- nil
	}()

	go func() {
		sub, err := nc.Subscribe(subjectTyping, func(msg *nats.Msg) {
			var event domain.TypingChanged
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal chat.typing event: %v", err)
				return
			}
			msgCtx, span := tracing.StartConsumeSpan(ctx, subjectTyping, msg.Header)
			handlers.OnTypingChanged(msgCtx, event)
			span.End()
		})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = sub.Unsubscribe() }()

		<-ctx.Done()
		errCh <- nil
	}()

	for range 7 {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func consumeOne(ctx context.Context, js jetstream.JetStream, streamName, filterSubject string, handle func(ctx context.Context, data []byte)) error {
	return consumeMulti(ctx, js, streamName, []string{filterSubject}, handle)
}

func consumeMulti(ctx context.Context, js jetstream.JetStream, streamName string, filterSubjects []string, handle func(ctx context.Context, data []byte)) error {
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return err
	}

	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects: filterSubjects,
		AckPolicy:      jetstream.AckNonePolicy,
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
			log.Printf("ws-gateway: failed to fetch %v batch: %v", filterSubjects, err)
			continue
		}

		for msg := range msgs.Messages() {
			msgCtx, span := tracing.StartConsumeSpan(ctx, msg.Subject(), msg.Headers())
			handle(msgCtx, msg.Data())
			span.End()
		}
	}
}
