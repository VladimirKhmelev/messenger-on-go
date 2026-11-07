package hub

import (
	"context"
	"fmt"

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

func (f *Fanout) HandleMessageCreated(ctx context.Context, event domain.MessageCreated) error {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list members for chat %s: %w", event.ChatID, err)
	}

	message, err := f.messages.GetMessage(ctx, event.MessageID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to fetch message %s for fanout: %w", event.MessageID, err)
	}

	return f.broadcast(ctx, userIDs, "", MessageReceived{ChatID: event.ChatID, Message: message})
}

func (f *Fanout) HandleMessageUpdated(ctx context.Context, event domain.MessageUpdated) error {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list members for chat %s: %w", event.ChatID, err)
	}

	return f.broadcast(ctx, userIDs, "", event)
}

func (f *Fanout) HandleMessageRead(ctx context.Context, event domain.MessageRead) error {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list members for chat %s: %w", event.ChatID, err)
	}

	return f.broadcast(ctx, userIDs, event.UserID, event)
}

func (f *Fanout) HandleNotifyPush(ctx context.Context, event domain.NotifyPush) error {
	return f.registry.Broadcast(ctx, event.UserID, event)
}

func (f *Fanout) HandlePresenceChanged(ctx context.Context, event domain.PresenceChanged) error {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list contacts for presence fanout of %s: %w", event.UserID, err)
	}

	return f.broadcast(ctx, contacts, "", event)
}

func (f *Fanout) HandleTypingChanged(ctx context.Context, event domain.TypingChanged) error {
	userIDs, err := f.members.ListMembers(ctx, event.ChatID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list members for chat %s: %w", event.ChatID, err)
	}

	return f.broadcast(ctx, userIDs, event.UserID, event)
}

func (f *Fanout) HandleChatDeleted(ctx context.Context, event domain.ChatDeleted) error {
	return f.broadcast(ctx, event.MemberUserIDs, "", event)
}

func (f *Fanout) HandleProfileUpdated(ctx context.Context, event domain.ProfileUpdated) error {
	contacts, err := f.contacts.ListContacts(ctx, event.UserID)
	if err != nil {
		return fmt.Errorf("ws-gateway: failed to list contacts for profile fanout of %s: %w", event.UserID, err)
	}

	return f.broadcast(ctx, contacts, "", event)
}

func (f *Fanout) broadcast(ctx context.Context, userIDs []string, skipUserID string, notification any) error {
	var first error
	for _, userID := range userIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if skipUserID != "" && userID == skipUserID {
			continue
		}
		if err := f.registry.Broadcast(ctx, userID, notification); err != nil && first == nil {
			first = err
		}
	}
	return first
}
