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

type ContactsLister interface {
	ListContacts(ctx context.Context, userID string) ([]string, error)
}

type Fanout struct {
	registry *Registry
	members  MembersLister
	messages MessageGetter
	contacts ContactsLister
}

func NewFanout(registry *Registry, members MembersLister, messages MessageGetter, contacts ContactsLister) *Fanout {
	return &Fanout{registry: registry, members: members, messages: messages, contacts: contacts}
}

func (f *Fanout) HandleMessageCreated(ctx context.Context, event events.MessageCreated) {
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

func (f *Fanout) HandleMessageUpdated(ctx context.Context, event events.MessageUpdated) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	payload := serverMessage{
		Type:      "message_updated",
		ChatID:    event.ChatID,
		MessageID: event.MessageID,
		NewText:   event.NewBody,
		Deleted:   event.Deleted,
	}

	for _, userID := range userIDs {
		f.registry.Broadcast(userID, payload)
	}
}

func (f *Fanout) HandleNotifyPush(_ context.Context, event events.NotifyPush) {
	payload := serverMessage{
		Type:      "notify_push",
		ChatID:    event.ChatID,
		MessageID: event.MessageID,
	}

	f.registry.Broadcast(event.UserID, payload)
}

func (f *Fanout) HandlePresenceChanged(ctx context.Context, event events.PresenceChanged) {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		log.Printf("ws-gateway: failed to list contacts for presence fanout of %s: %v", event.UserID, err)
		return
	}

	payload := serverMessage{
		Type:         "presence_changed",
		PeerUserID:   event.UserID,
		Online:       event.Online,
		LastSeenUnix: event.LastSeenUnix,
	}

	for _, contactID := range contacts {
		f.registry.Broadcast(contactID, payload)
	}
}
