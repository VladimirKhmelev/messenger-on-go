package hub

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

	f.broadcast(userIDs, "", MessageReceived{ChatID: event.ChatID, Message: message})
}

func (f *Fanout) HandleMessageUpdated(ctx context.Context, event domain.MessageUpdated) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	f.broadcast(userIDs, "", event)
}

func (f *Fanout) HandleMessageRead(ctx context.Context, event domain.MessageRead) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	f.broadcast(userIDs, event.UserID, event)
}

func (f *Fanout) HandleNotifyPush(_ context.Context, event domain.NotifyPush) {
	f.registry.Broadcast(event.UserID, event)
}

func (f *Fanout) HandlePresenceChanged(ctx context.Context, event domain.PresenceChanged) {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		log.Printf("ws-gateway: failed to list contacts for presence fanout of %s: %v", event.UserID, err)
		return
	}

	f.broadcast(contacts, "", event)
}

func (f *Fanout) HandleTypingChanged(ctx context.Context, event domain.TypingChanged) {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		log.Printf("ws-gateway: failed to list members for chat %s: %v", event.ChatID, err)
		return
	}

	f.broadcast(userIDs, event.UserID, event)
}

func (f *Fanout) HandleChatDeleted(_ context.Context, event domain.ChatDeleted) {
	f.broadcast(event.MemberUserIDs, "", event)
}

func (f *Fanout) HandleProfileUpdated(ctx context.Context, event domain.ProfileUpdated) {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		log.Printf("ws-gateway: failed to list contacts for profile fanout of %s: %v", event.UserID, err)
		return
	}

	f.broadcast(contacts, "", event)
}

func (f *Fanout) broadcast(userIDs []string, skipUserID string, notification any) {
	for _, userID := range userIDs {
		if skipUserID != "" && userID == skipUserID {
			continue
		}
		f.registry.Broadcast(userID, notification)
	}
}
