package notify

import (
	"context"
	"encoding/json"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/events"
)

type ChatClient interface {
	ListMembers(ctx context.Context, chatID string) ([]string, error)
	IsOnline(ctx context.Context, userID string) (bool, error)
}

type Handler struct {
	chat ChatClient
}

func NewHandler(chat ChatClient) *Handler {
	return &Handler{chat: chat}
}

func (h *Handler) HandleMessageCreated(ctx context.Context, subject string, data []byte) {
	var event events.MessageCreated
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("notification-worker: failed to unmarshal %s: %v", subject, err)
		return
	}

	recipients, err := h.recipientsNeedingNotification(ctx, event)
	if err != nil {
		log.Printf("notification-worker: failed to resolve recipients for message %s: %v", event.MessageID, err)
		return
	}

	for _, userID := range recipients {
		log.Printf("notification-worker: user %s needs a push notification for message %s in chat %s",
			userID, event.MessageID, event.ChatID)
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
