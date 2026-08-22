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
	chatStreamName   = "CHAT_EVENTS"
	subjectMsg       = "msg.created"
	subjectMsgEdit   = "msg.updated"
	subjectMsgDelete = "msg.deleted"
	subjectMsgRead   = "msg.read"

	notifyStreamName = "NOTIFY_EVENTS"
	subjectNotify    = "notify.push"

	userStreamName        = "USER_EVENTS"
	subjectProfileUpdated = "user.profile_updated"

	subjectPresence = "user.presence"
	subjectTyping   = "chat.typing"

	pullMaxWait = 5 * time.Second
	pullBatch   = 10
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

type MessageRead struct {
	ChatID    string    `json:"chat_id"`
	UserID    string    `json:"user_id"`
	MessageID string    `json:"message_id"`
	ReadAt    time.Time `json:"read_at"`
}

type NotifyPush struct {
	UserID    string    `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}

type PresenceChanged struct {
	UserID       string `json:"user_id"`
	Online       bool   `json:"online"`
	LastSeenUnix int64  `json:"last_seen_unix"`
}

type ProfileUpdated struct {
	UserID      string `json:"user_id"`
	Tag         string `json:"tag"`
	DisplayName string `json:"display_name"`
}

type TypingChanged struct {
	ChatID string `json:"chat_id"`
	UserID string `json:"user_id"`
}

type Handlers struct {
	OnMessageCreated  func(ctx context.Context, event MessageCreated)
	OnMessageUpdated  func(ctx context.Context, event MessageUpdated)
	OnMessageRead     func(ctx context.Context, event MessageRead)
	OnNotifyPush      func(ctx context.Context, event NotifyPush)
	OnPresenceChanged func(ctx context.Context, event PresenceChanged)
	OnProfileUpdated  func(ctx context.Context, event ProfileUpdated)
	OnTypingChanged   func(ctx context.Context, event TypingChanged)
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

func (p *PresencePublisher) PublishPresenceChanged(event PresenceChanged) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.nc.Publish(subjectPresence, payload)
}

func (p *PresencePublisher) PublishTypingChanged(event TypingChanged) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.nc.Publish(subjectTyping, payload)
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

	errCh := make(chan error, 7)

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
		errCh <- consumeMulti(ctx, js, chatStreamName, []string{subjectMsgEdit, subjectMsgDelete}, func(ctx context.Context, data []byte) {
			var event MessageUpdated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.updated/deleted event: %v", err)
				return
			}
			handlers.OnMessageUpdated(ctx, event)
		})
	}()

	go func() {
		errCh <- consumeOne(ctx, js, chatStreamName, subjectMsgRead, func(ctx context.Context, data []byte) {
			var event MessageRead
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal msg.read event: %v", err)
				return
			}
			handlers.OnMessageRead(ctx, event)
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

	go func() {
		errCh <- consumeOne(ctx, js, userStreamName, subjectProfileUpdated, func(ctx context.Context, data []byte) {
			var event ProfileUpdated
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal user.profile_updated event: %v", err)
				return
			}
			handlers.OnProfileUpdated(ctx, event)
		})
	}()

	go func() {
		sub, err := nc.Subscribe(subjectPresence, func(msg *nats.Msg) {
			var event PresenceChanged
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal user.presence event: %v", err)
				return
			}
			handlers.OnPresenceChanged(ctx, event)
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
			var event TypingChanged
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				log.Printf("ws-gateway: failed to unmarshal chat.typing event: %v", err)
				return
			}
			handlers.OnTypingChanged(ctx, event)
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
			handle(ctx, msg.Data())
		}
	}
}
