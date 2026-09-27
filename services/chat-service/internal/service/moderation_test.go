package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func TestChatService_ReportMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	message, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "spam spam spam")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	if err := svc.ReportMessage(context.Background(), message.ID, "user-b", domain.ReportCategorySpam, "buy now"); err != nil {
		t.Fatalf("ReportMessage() unexpected error: %v", err)
	}

	reported, err := repo.HasReported(context.Background(), message.ID, "user-b")
	if err != nil {
		t.Fatalf("HasReported() unexpected error: %v", err)
	}
	if !reported {
		t.Error("HasReported() = false, want true after ReportMessage()")
	}
}

func TestChatService_ReportMessage_InvalidCategory(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	message, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.ReportMessage(context.Background(), message.ID, "user-b", domain.ReportCategory("bogus"), "")
	if !errors.Is(err, domain.ErrInvalidReportCategory) {
		t.Errorf("ReportMessage() error = %v, want %v", err, domain.ErrInvalidReportCategory)
	}
}

func TestChatService_ReportMessage_NotChatMember(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	message, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.ReportMessage(context.Background(), message.ID, "user-stranger", domain.ReportCategorySpam, "")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("ReportMessage() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}
