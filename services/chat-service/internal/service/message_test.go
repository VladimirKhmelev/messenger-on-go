package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func TestChatService_SendMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	message, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}
	if message.Body != "hello" {
		t.Errorf("SendMessage() body = %q, want %q", message.Body, "hello")
	}
}

func TestChatService_SendMessage_NotMember(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-stranger", "hello")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_SendMessage_BlockedInPrivateChat(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now(), ChatType: domain.ChatTypePrivate}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))
	_ = repo.BlockUser(context.Background(), "user-b", "user-a")

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if !errors.Is(err, domain.ErrUserBlocked) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrUserBlocked)
	}
}

func TestChatService_SendMessage_NotBlockedInGroupChat(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now(), ChatType: domain.ChatTypeGroup}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b", "user-c"))
	_ = repo.BlockUser(context.Background(), "user-b", "user-a")

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Errorf("SendMessage() in group chat unexpected error: %v", err)
	}
}

func TestChatService_SendMessage_Empty(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "")
	if !errors.Is(err, domain.ErrEmptyMessage) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrEmptyMessage)
	}
}

func TestChatService_GetHistory_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	if _, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hi"); err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	messages, err := svc.GetHistory(context.Background(), chat.ID, "user-b", 0, 0)
	if err != nil {
		t.Fatalf("GetHistory() unexpected error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("GetHistory() returned %d messages, want 1", len(messages))
	}
}

func TestChatService_GetHistory_NotMember(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.GetHistory(context.Background(), chat.ID, "user-stranger", 0, 0)
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("GetHistory() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_MarkRead_MessageFromDifferentChat(t *testing.T) {
	repo := newFakeChatRepository()
	chatA := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chatA, chatKeys("user-a", "user-b"))
	chatB := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chatB, chatKeys("user-a", "user-c"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	messageInB, err := svc.SendMessage(context.Background(), chatB.ID, "user-a", "hi")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.MarkRead(context.Background(), chatA.ID, "user-b", messageInB.ID)
	if !errors.Is(err, domain.ErrMessageNotInChat) {
		t.Errorf("MarkRead() error = %v, want %v", err, domain.ErrMessageNotInChat)
	}
}

func TestChatService_GetMessage_NotFound(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.GetMessage(context.Background(), "missing-message")
	if !errors.Is(err, domain.ErrMessageNotFound) {
		t.Errorf("GetMessage() error = %v, want %v", err, domain.ErrMessageNotFound)
	}
}

func TestChatService_EditMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker(), newFakeRateLimiter())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	edited, err := svc.EditMessage(context.Background(), sent.ID, "user-a", "hello, edited")
	if err != nil {
		t.Fatalf("EditMessage() unexpected error: %v", err)
	}
	if edited.Body != "hello, edited" {
		t.Errorf("EditMessage() body = %q, want %q", edited.Body, "hello, edited")
	}
	if edited.EditedAt == nil {
		t.Error("EditMessage() EditedAt = nil, want set")
	}

	if len(publisher.messageUpdatedEvents) != 1 {
		t.Fatalf("EditMessage() published %d msg.updated events, want 1", len(publisher.messageUpdatedEvents))
	}
	event := publisher.messageUpdatedEvents[0]
	if event.Deleted || event.NewBody == nil || *event.NewBody != "hello, edited" {
		t.Errorf("EditMessage() published event = %+v, want NewBody=hello, edited, Deleted=false", event)
	}

	stored, err := svc.GetMessage(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("GetMessage() unexpected error: %v", err)
	}
	if stored.Body != "hello, edited" {
		t.Errorf("GetMessage() after edit body = %q, want %q", stored.Body, "hello, edited")
	}
}

func TestChatService_EditMessage_NotSender(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	_, err = svc.EditMessage(context.Background(), sent.ID, "user-b", "hijacked")
	if !errors.Is(err, domain.ErrNotMessageSender) {
		t.Errorf("EditMessage() error = %v, want %v", err, domain.ErrNotMessageSender)
	}
}

func TestChatService_DeleteMessageForAll_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker(), newFakeRateLimiter())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	if err := svc.DeleteMessageForAll(context.Background(), sent.ID, "user-a"); err != nil {
		t.Fatalf("DeleteMessageForAll() unexpected error: %v", err)
	}

	if len(publisher.messageUpdatedEvents) != 1 || !publisher.messageUpdatedEvents[0].Deleted {
		t.Fatalf("DeleteMessageForAll() published events = %+v, want 1 deleted event", publisher.messageUpdatedEvents)
	}

	stored, err := svc.GetMessage(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("GetMessage() unexpected error: %v", err)
	}
	if stored.DeletedAt == nil {
		t.Error("GetMessage() after delete DeletedAt = nil, want set")
	}

	messages, err := repo.ListMessages(context.Background(), chat.ID, "user-b", 10, 0)
	if err != nil {
		t.Fatalf("ListMessages() unexpected error: %v", err)
	}
	if len(messages) != 1 || messages[0].DeletedAt == nil {
		t.Errorf("ListMessages() after delete-for-all = %+v, want 1 tombstoned message", messages)
	}
}

func TestChatService_DeleteMessageForMe_NotChatMember(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.DeleteMessageForMe(context.Background(), sent.ID, "user-stranger")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("DeleteMessageForMe() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_EditMessage_EmptyBody(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	_, err = svc.EditMessage(context.Background(), sent.ID, "user-a", "")
	if !errors.Is(err, domain.ErrEmptyMessage) {
		t.Errorf("EditMessage() error = %v, want %v", err, domain.ErrEmptyMessage)
	}
}
