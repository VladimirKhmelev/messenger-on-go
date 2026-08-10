package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
)

const HistoryDefaultLimit = 50

type AuthClient interface {
	UserExists(ctx context.Context, bearerToken, userID string) (bool, error)
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event events.MessageCreated) error
	PublishMessageUpdated(ctx context.Context, event events.MessageUpdated) error
}

type PresenceChecker interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
	LastSeen(ctx context.Context, userID string) (int64, error)
	SetOffline(ctx context.Context, userID string) error
}

type ChatService struct {
	chats    repository.ChatRepository
	auth     AuthClient
	events   EventPublisher
	presence PresenceChecker
}

func NewChatService(chats repository.ChatRepository, auth AuthClient, eventPublisher EventPublisher, presence PresenceChecker) *ChatService {
	return &ChatService{chats: chats, auth: auth, events: eventPublisher, presence: presence}
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

	if err := s.events.PublishMessageCreated(ctx, events.MessageCreated{
		MessageID: message.ID,
		ChatID:    message.ChatID,
		SenderID:  message.SenderID,
		CreatedAt: message.CreatedAt,
	}); err != nil {
		log.Printf("chat-service: failed to publish msg.created event for %s: %v", message.ID, err)
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

	return s.chats.ListMessages(ctx, chatID, requesterID, limit)
}

func (s *ChatService) EditMessage(ctx context.Context, messageID, requesterID, newBody string) (*domain.Message, error) {
	if newBody == "" {
		return nil, domain.ErrEmptyMessage
	}

	message, err := s.chats.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if message.DeletedAt != nil {
		return nil, domain.ErrMessageDeleted
	}
	if message.SenderID != requesterID {
		return nil, domain.ErrNotMessageSender
	}

	now := time.Now()
	if err := s.chats.AppendMessageEvent(ctx, &domain.MessageEvent{
		ID:        uuid.NewString(),
		MessageID: messageID,
		ChatID:    message.ChatID,
		ActorID:   requesterID,
		Type:      domain.MessageEventEdited,
		NewBody:   &newBody,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	if err := s.events.PublishMessageUpdated(ctx, events.MessageUpdated{
		MessageID: messageID,
		ChatID:    message.ChatID,
		NewBody:   &newBody,
		UpdatedAt: now,
	}); err != nil {
		log.Printf("chat-service: failed to publish msg.updated event for %s: %v", messageID, err)
	}

	message.Body = newBody
	message.EditedAt = &now
	return message, nil
}

func (s *ChatService) DeleteMessageForAll(ctx context.Context, messageID, requesterID string) error {
	message, err := s.chats.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if message.DeletedAt != nil {
		return nil
	}
	if message.SenderID != requesterID {
		return domain.ErrNotMessageSender
	}

	now := time.Now()
	if err := s.chats.AppendMessageEvent(ctx, &domain.MessageEvent{
		ID:        uuid.NewString(),
		MessageID: messageID,
		ChatID:    message.ChatID,
		ActorID:   requesterID,
		Type:      domain.MessageEventDeletedForAll,
		CreatedAt: now,
	}); err != nil {
		return err
	}

	if err := s.events.PublishMessageUpdated(ctx, events.MessageUpdated{
		MessageID: messageID,
		ChatID:    message.ChatID,
		Deleted:   true,
		UpdatedAt: now,
	}); err != nil {
		log.Printf("chat-service: failed to publish msg.deleted event for %s: %v", messageID, err)
	}

	return nil
}

func (s *ChatService) DeleteMessageForMe(ctx context.Context, messageID, requesterID string) error {
	message, err := s.chats.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}

	isMember, err := s.chats.IsMember(ctx, message.ChatID, requesterID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrNotChatMember
	}

	return s.chats.HideMessageForUser(ctx, messageID, requesterID)
}

type ChatSummary struct {
	ChatID        string
	MemberUserIDs []string
	LastMessage   *domain.Message
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

		summaries = append(summaries, &ChatSummary{
			ChatID:        chat.ID,
			MemberUserIDs: memberIDs,
			LastMessage:   lastMessage,
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

func (s *ChatService) GetMessage(ctx context.Context, messageID string) (*domain.Message, error) {
	return s.chats.GetMessage(ctx, messageID)
}

func (s *ChatService) GetPresence(ctx context.Context, userID string) (online bool, lastSeenUnix int64, err error) {
	online, err = s.presence.IsOnline(ctx, userID)
	if err != nil {
		return false, 0, err
	}

	lastSeenUnix, err = s.presence.LastSeen(ctx, userID)
	if err != nil {
		return false, 0, err
	}

	return online, lastSeenUnix, nil
}

func (s *ChatService) SetOffline(ctx context.Context, userID string) error {
	return s.presence.SetOffline(ctx, userID)
}
