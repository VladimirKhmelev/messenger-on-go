package ws

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

type ChatClient interface {
	SendMessage(ctx context.Context, bearerToken, chatID, text string) (string, error)
	GetHistory(ctx context.Context, bearerToken, chatID string, limit, offset int32) ([]domain.Message, error)
	GetPresence(ctx context.Context, userID string) (online bool, lastSeenUnix int64, err error)
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	EditMessage(ctx context.Context, bearerToken, chatID, messageID, text string) error
	DeleteMessageForAll(ctx context.Context, bearerToken, chatID, messageID string) error
	DeleteMessageForMe(ctx context.Context, bearerToken, chatID, messageID string) error
	MarkRead(ctx context.Context, bearerToken, chatID, messageID string) error
	GetReadStatus(ctx context.Context, chatID, userID string) (string, error)
	SetTyping(ctx context.Context, chatID, userID string) error
}

type PresencePublisher interface {
	PublishPresenceChanged(ctx context.Context, event domain.PresenceChanged) error
	PublishTypingChanged(ctx context.Context, event domain.TypingChanged) error
}
