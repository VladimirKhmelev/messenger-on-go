package notify

import (
	"context"
	"encoding/json"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/authclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/webpush"
)

type ChatClient interface {
	ListMembers(ctx context.Context, chatID string) ([]string, error)
	IsOnline(ctx context.Context, userID string) (bool, error)
}

type AuthClient interface {
	ListPushSubscriptions(ctx context.Context, userID string) ([]authclient.PushSubscription, error)
}

type WebPushSender interface {
	Send(ctx context.Context, sub webpush.Subscription, payload webpush.Payload)
}

type EventPublisher interface {
	PublishNotifyPush(ctx context.Context, event events.NotifyPush) error
}

type Handler struct {
	chat    ChatClient
	auth    AuthClient
	webpush WebPushSender
	events  EventPublisher
}

func NewHandler(chat ChatClient, auth AuthClient, webpushSender WebPushSender, eventPublisher EventPublisher) *Handler {
	return &Handler{chat: chat, auth: auth, webpush: webpushSender, events: eventPublisher}
}

func (h *Handler) HandleMessageCreated(ctx context.Context, subject string, data []byte) {
	var event events.MessageCreated
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("notification-worker: failed to unmarshal %s: %v", subject, err)
		metrics.NATSConsumeErrorsTotal.Inc()
		return
	}

	recipients, err := h.recipientsNeedingNotification(ctx, event)
	if err != nil {
		log.Printf("notification-worker: failed to resolve recipients for message %s: %v", event.MessageID, err)
		metrics.NATSConsumeErrorsTotal.Inc()
		return
	}

	for _, userID := range recipients {
		if err := h.events.PublishNotifyPush(ctx, events.NotifyPush{
			UserID:    userID,
			ChatID:    event.ChatID,
			MessageID: event.MessageID,
			CreatedAt: event.CreatedAt,
		}); err != nil {
			log.Printf("notification-worker: failed to publish notify.push for user %s: %v", userID, err)
			metrics.NATSConsumeErrorsTotal.Inc()
		}

		h.sendWebPush(ctx, userID, event)
	}
}

func (h *Handler) sendWebPush(ctx context.Context, userID string, event events.MessageCreated) {
	subs, err := h.auth.ListPushSubscriptions(ctx, userID)
	if err != nil {
		log.Printf("notification-worker: failed to list push subscriptions for user %s: %v", userID, err)
		return
	}

	payload := webpush.Payload{ChatID: event.ChatID, MessageID: event.MessageID}
	for _, sub := range subs {
		h.webpush.Send(ctx, webpush.Subscription{
			Endpoint:  sub.Endpoint,
			P256dhKey: sub.P256dhKey,
			AuthKey:   sub.AuthKey,
		}, payload)
	}
}

func (h *Handler) recipientsNeedingNotification(ctx context.Context, event events.MessageCreated) ([]string, error) {
	memberIDs, err := h.chat.ListMembers(ctx, event.ChatID)
	if err != nil {
		return nil, err
	}

	var recipients []string
	for _, userID := range memberIDs {
		if userID == event.SenderID {
			continue
		}

		online, err := h.chat.IsOnline(ctx, userID)
		if err != nil {
			log.Printf("notification-worker: failed to check presence for user %s: %v", userID, err)
			continue
		}
		if !online {
			recipients = append(recipients, userID)
		}
	}
	return recipients, nil
}

func (h *Handler) HandleUserEvent(_ context.Context, subject string, data []byte) {
	log.Printf("notification-worker: received %s: %s", subject, string(data))
}
