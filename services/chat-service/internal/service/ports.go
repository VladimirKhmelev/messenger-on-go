package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

type ChatRepository interface {
	CreateChat(ctx context.Context, chat *domain.Chat, chatKeyByUserID map[string]domain.MemberChatKey) error
	GetChat(ctx context.Context, chatID string) (*domain.Chat, error)
	DeleteChat(ctx context.Context, chatID string) error
	FindPrivateChat(ctx context.Context, userA, userB string) (*domain.Chat, error)
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
	IsAdmin(ctx context.Context, chatID, userID string) (bool, error)
	GetMember(ctx context.Context, chatID, userID string) (*domain.ChatMember, error)
	MemberCount(ctx context.Context, chatID string) (int, error)
	AddMember(ctx context.Context, chatID, userID string, key domain.MemberChatKey) error
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

	UpsertChatAvatar(ctx context.Context, avatar *domain.ChatAvatar) error
	GetChatAvatar(ctx context.Context, chatID string) (*domain.ChatAvatar, error)

	BlockUser(ctx context.Context, blockerID, blockedID string) error
	UnblockUser(ctx context.Context, blockerID, blockedID string) error
	IsBlocked(ctx context.Context, userA, userB string) (bool, error)
	ListBlockedUsers(ctx context.Context, blockerID string) ([]*domain.BlockedUser, error)

	CreateMessageReport(ctx context.Context, report *domain.MessageReport) error
	HasReported(ctx context.Context, messageID, reporterID string) (bool, error)
}

type AuthClient interface {
	UserExists(ctx context.Context, bearerToken, userID string) (bool, error)
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event domain.MessageCreated) error
	PublishMessageUpdated(ctx context.Context, event domain.MessageUpdated) error
	PublishMessageRead(ctx context.Context, event domain.MessageRead) error
	PublishChatDeleted(ctx context.Context, event domain.ChatDeleted) error
}

type PresenceChecker interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
	LastSeen(ctx context.Context, userID string) (int64, error)
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	SetTyping(ctx context.Context, chatID, userID string) error
	IsTyping(ctx context.Context, chatID, userID string) (bool, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, userID string) (bool, error)
}
