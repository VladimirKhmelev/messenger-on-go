package service

import (
	"context"
	"errors"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

const (
	HistoryDefaultLimit = 50

	MaxMessageBodyBytes = 64 * 1024
)

type ChatService struct {
	chats       ChatRepository
	auth        AuthClient
	events      EventPublisher
	presence    PresenceChecker
	sendLimiter RateLimiter
}

func NewChatService(chats ChatRepository, auth AuthClient, eventPublisher EventPublisher, presence PresenceChecker, sendLimiter RateLimiter) *ChatService {
	return &ChatService{chats: chats, auth: auth, events: eventPublisher, presence: presence, sendLimiter: sendLimiter}
}

func (s *ChatService) CreateChat(ctx context.Context, bearerToken, requesterID, targetID string, encryptedChatKeyByUserID, wrappedForPublicKeyByUserID map[string]string) (*domain.Chat, error) {
	isSelfChat := requesterID == targetID

	if !isSelfChat {
		exists, err := s.auth.UserExists(ctx, bearerToken, targetID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, domain.ErrTargetUserNotFound
		}

		blocked, err := s.chats.IsBlocked(ctx, requesterID, targetID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, domain.ErrUserBlocked
		}
	}

	existing, err := s.chats.FindPrivateChat(ctx, requesterID, targetID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrChatNotFound) {
		return nil, err
	}

	if encryptedChatKeyByUserID[requesterID] == "" || encryptedChatKeyByUserID[targetID] == "" ||
		wrappedForPublicKeyByUserID[requesterID] == "" || wrappedForPublicKeyByUserID[targetID] == "" {
		return nil, domain.ErrMissingChatKey
	}

	chat := &domain.Chat{
		ID:        uuid.NewString(),
		CreatedAt: time.Now(),
		ChatType:  domain.ChatTypePrivate,
	}

	chatKeyByUserID := map[string]domain.MemberChatKey{
		requesterID: {EncryptedChatKey: encryptedChatKeyByUserID[requesterID], WrappedForPublicKey: wrappedForPublicKeyByUserID[requesterID]},
		targetID:    {EncryptedChatKey: encryptedChatKeyByUserID[targetID], WrappedForPublicKey: wrappedForPublicKeyByUserID[targetID]},
	}

	if err := s.chats.CreateChat(ctx, chat, chatKeyByUserID); err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *ChatService) GetChatKey(ctx context.Context, chatID, requesterID string) (string, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return "", err
	}
	if !isMember {
		return "", domain.ErrNotChatMember
	}

	return s.chats.GetChatKeyForUser(ctx, chatID, requesterID)
}

func (s *ChatService) ListChatKeys(ctx context.Context, chatID, requesterID string) ([]*domain.ChatMember, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	return s.chats.ListMembers(ctx, chatID)
}

func (s *ChatService) UpdateChatKey(ctx context.Context, chatID, requesterID, targetUserID, encryptedChatKey, wrappedForPublicKey string) error {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrNotChatMember
	}

	if encryptedChatKey == "" || wrappedForPublicKey == "" {
		return domain.ErrMissingChatKey
	}

	return s.chats.UpdateChatKey(ctx, chatID, targetUserID, encryptedChatKey, wrappedForPublicKey)
}

type ChatSummary struct {
	ChatID        string
	MemberUserIDs []string
	LastMessage   *domain.Message
	ChatType      domain.ChatType
	Name          string
}

func (s *ChatService) ListChats(ctx context.Context, userID string) ([]*ChatSummary, error) {
	chats, err := s.chats.ListChatsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	summaries := make([]*ChatSummary, 0, len(chats))
	for _, chat := range chats {
		members, err := s.chats.ListMembers(ctx, chat.ID)
		if err != nil {
			return nil, err
		}
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.UserID)
		}

		lastMessage, err := s.chats.GetLastMessage(ctx, chat.ID, userID)
		if err != nil {
			return nil, err
		}

		name := ""
		if chat.Name != nil {
			name = *chat.Name
		}

		summaries = append(summaries, &ChatSummary{
			ChatID:        chat.ID,
			MemberUserIDs: memberIDs,
			LastMessage:   lastMessage,
			ChatType:      chat.ChatType,
			Name:          name,
		})
	}

	return summaries, nil
}

func (s *ChatService) ListContacts(ctx context.Context, userID string) ([]string, error) {
	chats, err := s.chats.ListChatsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	contacts := make([]string, 0, len(chats))
	for _, chat := range chats {
		members, err := s.chats.ListMembers(ctx, chat.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.UserID == userID || seen[m.UserID] {
				continue
			}
			seen[m.UserID] = true
			contacts = append(contacts, m.UserID)
		}
	}

	return contacts, nil
}

func (s *ChatService) ListChatMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error) {
	return s.chats.ListMembers(ctx, chatID)
}

func (s *ChatService) ListMembersForUser(ctx context.Context, chatID, requesterID string) ([]*domain.ChatMember, *domain.Chat, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return nil, nil, err
	}
	if !isMember {
		return nil, nil, domain.ErrNotChatMember
	}

	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, nil, err
	}

	members, err := s.chats.ListMembers(ctx, chatID)
	if err != nil {
		return nil, nil, err
	}

	return members, chat, nil
}

func (s *ChatService) ListMembers(ctx context.Context, chatID string) ([]string, error) {
	members, err := s.chats.ListMembers(ctx, chatID)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}
	return userIDs, nil
}
