package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func (s *ChatService) ReportMessage(ctx context.Context, messageID, reporterID string, category domain.ReportCategory, comment string) error {
	if err := validateReportCategory(category); err != nil {
		return err
	}

	message, err := s.chats.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}

	isMember, err := s.chats.IsMember(ctx, message.ChatID, reporterID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrNotChatMember
	}

	alreadyReported, err := s.chats.HasReported(ctx, messageID, reporterID)
	if err != nil {
		return err
	}
	if alreadyReported {
		return domain.ErrAlreadyReported
	}

	return s.chats.CreateMessageReport(ctx, &domain.MessageReport{
		ID:         uuid.NewString(),
		MessageID:  messageID,
		ChatID:     message.ChatID,
		ReporterID: reporterID,
		Category:   category,
		Comment:    comment,
		CreatedAt:  time.Now(),
	})
}

func (s *ChatService) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return domain.ErrCannotBlockSelf
	}
	return s.chats.BlockUser(ctx, blockerID, blockedID)
}

func (s *ChatService) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	return s.chats.UnblockUser(ctx, blockerID, blockedID)
}

func (s *ChatService) ListBlockedUsers(ctx context.Context, blockerID string) ([]*domain.BlockedUser, error) {
	return s.chats.ListBlockedUsers(ctx, blockerID)
}
