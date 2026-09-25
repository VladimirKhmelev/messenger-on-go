package ws

import (
	"context"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

type Fanout struct {
	registry *Registry
	members  MembersLister
	messages MessageGetter
	contacts ContactsLister
}

func NewFanout(registry *Registry, members MembersLister, messages MessageGetter, contacts ContactsLister) *Fanout {
	return &Fanout{registry: registry, members: members, messages: messages, contacts: contacts}
}

func (f *Fanout) HandleMessageCreated(ctx context.Context, event domain.MessageCreated) {
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
	wire := toWireMessage(message)

	payload := serverMessage{
		Type:    "message_received",
		ChatID:  event.ChatID,
		Message: &wire,
	}

	for _, userID := range userIDs {
		f.registry.Broadcast(userID, payload)
	}
}

func (f *Fanout) HandleMessageUpdated(ctx context.Context, event domain.MessageUpdated) {
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

func (f *Fanout) HandleMessageRead(ctx context.Context, event domain.MessageRead) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	payload := serverMessage{
		Type:              "read_status",
		ChatID:            event.ChatID,
		PeerUserID:        event.UserID,
		LastReadMessageID: event.MessageID,
	}

	for _, userID := range userIDs {
		if userID == event.UserID {
			continue
		}
		f.registry.Broadcast(userID, payload)
	}
}

func (f *Fanout) HandleNotifyPush(_ context.Context, event domain.NotifyPush) {
	payload := serverMessage{
		Type:      "notify_push",
		ChatID:    event.ChatID,
		MessageID: event.MessageID,
	}

	f.registry.Broadcast(event.UserID, payload)
}

func (f *Fanout) HandlePresenceChanged(ctx context.Context, event domain.PresenceChanged) {
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

func (f *Fanout) HandleTypingChanged(ctx context.Context, event domain.TypingChanged) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	payload := serverMessage{
		Type:       "typing_changed",
		ChatID:     event.ChatID,
		PeerUserID: event.UserID,
	}

	for _, userID := range userIDs {
		if userID == event.UserID {
			continue
		}
		f.registry.Broadcast(userID, payload)
	}
}

func (f *Fanout) HandleChatDeleted(_ context.Context, event domain.ChatDeleted) {
	payload := serverMessage{
		Type:   "chat_deleted",
		ChatID: event.ChatID,
	}

	for _, userID := range event.MemberUserIDs {
		f.registry.Broadcast(userID, payload)
	}
}

func (f *Fanout) HandleProfileUpdated(ctx context.Context, event domain.ProfileUpdated) {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		log.Printf("ws-gateway: failed to list contacts for profile fanout of %s: %v", event.UserID, err)
		return
	}

	payload := serverMessage{
		Type:            "profile_updated",
		PeerUserID:      event.UserID,
		PeerTag:         event.Tag,
		PeerDisplayName: event.DisplayName,
	}

	for _, contactID := range contacts {
		f.registry.Broadcast(contactID, payload)
	}
}
