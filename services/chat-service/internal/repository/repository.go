package repository

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

type MemberChatKey struct {
	EncryptedChatKey    string
	WrappedForPublicKey string
}

type ChatRepository interface {
	CreateChat(ctx context.Context, chat *domain.Chat, chatKeyByUserID map[string]MemberChatKey) error
	GetChat(ctx context.Context, chatID string) (*domain.Chat, error)
	FindPrivateChat(ctx context.Context, userA, userB string) (*domain.Chat, error)
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
	IsAdmin(ctx context.Context, chatID, userID string) (bool, error)
	GetMember(ctx context.Context, chatID, userID string) (*domain.ChatMember, error)
	MemberCount(ctx context.Context, chatID string) (int, error)
	AddMember(ctx context.Context, chatID, userID string, key MemberChatKey) error
	RemoveMember(ctx context.Context, chatID, userID string) error
	SetRole(ctx context.Context, chatID, userID string, role domain.MemberRole) error
	ListMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error)
	ListChatsForUser(ctx context.Context, userID string) ([]*domain.Chat, error)
	MarkRead(ctx context.Context, chatID, userID, messageID string, readAt time.Time) error
	GetChatKeyForUser(ctx context.Context, chatID, userID string) (string, error)
	UpdateChatKey(ctx context.Context, chatID, userID, encryptedChatKey, wrappedForPublicKey string) error

	CreateMessage(ctx context.Context, message *domain.Message) error
	ListMessages(ctx context.Context, chatID, requesterID string, limit, offset int) ([]*domain.Message, error)
	GetMessage(ctx context.Context, messageID string) (*domain.Message, error)
	GetLastMessage(ctx context.Context, chatID, requesterID string) (*domain.Message, error)

	AppendMessageEvent(ctx context.Context, event *domain.MessageEvent) error
	HideMessageForUser(ctx context.Context, messageID, userID string) error
}
