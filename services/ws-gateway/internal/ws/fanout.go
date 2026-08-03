package ws

import (
	"context"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

type MembersLister interface {
	ListMembers(ctx context.Context, chatID string) ([]string, error)
}

type MessageGetter interface {
	GetMessage(ctx context.Context, messageID string) (chatclient.Message, error)
}

type Fanout struct {
	registry *Registry
	members  MembersLister
	messages MessageGetter
}

func NewFanout(registry *Registry, members MembersLister, messages MessageGetter) *Fanout {
	return &Fanout{registry: registry, members: members, messages: messages}
}

func (f *Fanout) Handle(ctx context.Context, event events.MessageCreated) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	message, err := f.messages.GetMessage(ctx, event.MessageID)
	if err != nil {
		log.Printf("ws-gateway: failed to fetch message %s for fanout: %v", event.MessageID, err)
		return
	}

	payload := serverMessage{
		Type:    "message_received",
		ChatID:  event.ChatID,
		Message: &message,
	}

	for _, userID := range userIDs {
		f.registry.Broadcast(userID, payload)
	}
}
