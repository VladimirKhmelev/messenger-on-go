package repository

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

type ChatRepository interface {
	CreateChat(ctx context.Context, chat *domain.Chat, memberIDs []string) error
	GetChat(ctx context.Context, chatID string) (*domain.Chat, error)
	FindPrivateChat(ctx context.Context, userA, userB string) (*domain.Chat, error)
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
	ListMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error)
	ListChatsForUser(ctx context.Context, userID string) ([]*domain.Chat, error)

	CreateMessage(ctx context.Context, message *domain.Message) error
	ListMessages(ctx context.Context, chatID string, limit int) ([]*domain.Message, error)
	GetMessage(ctx context.Context, messageID string) (*domain.Message, error)
}
