package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID, body string) (*domain.Message, error) {
	if err := validateMessageBody(body); err != nil {
		return nil, err
	}

	allowed, err := s.sendLimiter.Allow(ctx, senderID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrTooManyMessages
	}

	isMember, err := s.chats.IsMember(ctx, chatID, senderID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	if err := s.checkNotBlockedInPrivateChat(ctx, chatID, senderID); err != nil {
		return nil, err
	}

	message := &domain.Message{
		ID:        uuid.NewString(),
		ChatID:    chatID,
		SenderID:  senderID,
		Body:      body,
		CreatedAt: time.Now(),
	}

	if err := s.chats.WithTx(ctx, func(ctx context.Context) error {
		if err := s.chats.CreateMessage(ctx, message); err != nil {
			return err
		}
		return s.events.PublishMessageCreated(ctx, domain.MessageCreated{
			MessageID: message.ID,
			ChatID:    message.ChatID,
			SenderID:  message.SenderID,
			CreatedAt: message.CreatedAt,
		})
	}); err != nil {
		return nil, err
	}
	metrics.MessagesSentTotal.Inc()

	return message, nil
}

func (s *ChatService) checkNotBlockedInPrivateChat(ctx context.Context, chatID, senderID string) error {
	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypePrivate {
		return nil
	}

	members, err := s.chats.ListMembers(ctx, chatID)
	if err != nil {
		return err
	}

	var otherID string
	for _, m := range members {
		if m.UserID != senderID {
			otherID = m.UserID
			break
		}
	}
	if otherID == "" {
		return nil
	}

	blocked, err := s.chats.IsBlocked(ctx, senderID, otherID)
	if err != nil {
		return err
	}
	if blocked {
		return domain.ErrUserBlocked
	}
	return nil
}

func (s *ChatService) GetHistory(ctx context.Context, chatID, requesterID string, limit, offset int) ([]*domain.Message, error) {
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
	if offset < 0 {
		offset = 0
	}

	return s.chats.ListMessages(ctx, chatID, requesterID, limit, offset)
}

func (s *ChatService) EditMessage(ctx context.Context, messageID, requesterID, newBody string) (*domain.Message, error) {
	if err := validateMessageBody(newBody); err != nil {
		return nil, err
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
	if err := s.chats.WithTx(ctx, func(ctx context.Context) error {
		if err := s.chats.AppendMessageEvent(ctx, &domain.MessageEvent{
			ID:        uuid.NewString(),
			MessageID: messageID,
			ChatID:    message.ChatID,
			ActorID:   requesterID,
			Type:      domain.MessageEventEdited,
			NewBody:   &newBody,
			CreatedAt: now,
		}); err != nil {
			return err
		}
		return s.events.PublishMessageUpdated(ctx, domain.MessageUpdated{
			MessageID: messageID,
			ChatID:    message.ChatID,
			NewBody:   &newBody,
			UpdatedAt: now,
		})
	}); err != nil {
		return nil, err
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
		isAdmin, err := s.chats.IsAdmin(ctx, message.ChatID, requesterID)
		if err != nil {
			return err
		}
		if !isAdmin {
			return domain.ErrNotMessageSender
		}
	}

	now := time.Now()
	return s.chats.WithTx(ctx, func(ctx context.Context) error {
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
		return s.events.PublishMessageUpdated(ctx, domain.MessageUpdated{
			MessageID: messageID,
			ChatID:    message.ChatID,
			Deleted:   true,
			UpdatedAt: now,
		})
	})
}

func (s *ChatService) MarkRead(ctx context.Context, chatID, requesterID, messageID string) error {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrNotChatMember
	}

	message, err := s.chats.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if message.ChatID != chatID {
		return domain.ErrMessageNotInChat
	}

	now := time.Now()
	return s.chats.WithTx(ctx, func(ctx context.Context) error {
		if err := s.chats.MarkRead(ctx, chatID, requesterID, messageID, now); err != nil {
			return err
		}
		return s.events.PublishMessageRead(ctx, domain.MessageRead{
			ChatID:    chatID,
			UserID:    requesterID,
			MessageID: messageID,
			ReadAt:    now,
		})
	})
}

func (s *ChatService) GetReadStatus(ctx context.Context, chatID, userID string) (string, error) {
	members, err := s.chats.ListMembers(ctx, chatID)
	if err != nil {
		return "", err
	}

	for _, m := range members {
		if m.UserID == userID {
			if m.LastReadMessageID == nil {
				return "", nil
			}
			return *m.LastReadMessageID, nil
		}
	}

	return "", nil
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

func (s *ChatService) GetMessage(ctx context.Context, messageID string) (*domain.Message, error) {
	return s.chats.GetMessage(ctx, messageID)
}
