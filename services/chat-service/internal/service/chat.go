package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
)

const HistoryDefaultLimit = 50

type AuthClient interface {
	UserExists(ctx context.Context, bearerToken, userID string) (bool, error)
}

type ChatService struct {
	chats repository.ChatRepository
	auth  AuthClient
}

func NewChatService(chats repository.ChatRepository, auth AuthClient) *ChatService {
	return &ChatService{chats: chats, auth: auth}
}

func (s *ChatService) CreateChat(ctx context.Context, bearerToken, requesterID, targetID string) (*domain.Chat, error) {
	if requesterID == targetID {
		return nil, domain.ErrCannotChatWithSelf
	}

	exists, err := s.auth.UserExists(ctx, bearerToken, targetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrTargetUserNotFound
	}

	existing, err := s.chats.FindPrivateChat(ctx, requesterID, targetID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrChatNotFound) {
		return nil, err
	}

	chat := &domain.Chat{
		ID:        uuid.NewString(),
		CreatedAt: time.Now(),
	}

	if err := s.chats.CreateChat(ctx, chat, []string{requesterID, targetID}); err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID, body string) (*domain.Message, error) {
	if body == "" {
		return nil, domain.ErrEmptyMessage
	}

	isMember, err := s.chats.IsMember(ctx, chatID, senderID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	message := &domain.Message{
		ID:        uuid.NewString(),
		ChatID:    chatID,
		SenderID:  senderID,
		Body:      body,
		CreatedAt: time.Now(),
	}

	if err := s.chats.CreateMessage(ctx, message); err != nil {
		return nil, err
	}

	return message, nil
}

func (s *ChatService) GetHistory(ctx context.Context, chatID, requesterID string, limit int) ([]*domain.Message, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	if limit <= 0 {
		limit = HistoryDefaultLimit
	}

	return s.chats.ListMessages(ctx, chatID, limit)
}
