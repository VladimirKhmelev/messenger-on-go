package notify

import (
	"context"
	"encoding/json"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/events"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) HandleMessageCreated(_ context.Context, subject string, data []byte) {
	var event events.MessageCreated
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("notification-worker: failed to unmarshal %s: %v", subject, err)
		return
	}
	log.Printf("notification-worker: received %s: message_id=%s chat_id=%s sender_id=%s",
		subject, event.MessageID, event.ChatID, event.SenderID)
}

func (h *Handler) HandleUserEvent(_ context.Context, subject string, data []byte) {
	log.Printf("notification-worker: received %s: %s", subject, string(data))
}
