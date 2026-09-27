package hub

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

type MembersLister interface {
	ListMembers(ctx context.Context, chatID string) ([]string, error)
}

type MessageGetter interface {
	GetMessage(ctx context.Context, messageID string) (domain.Message, error)
}

type ContactsLister interface {
	ListContacts(ctx context.Context, userID string) ([]string, error)
}

type Client interface {
	Deliver(notification any)
}
