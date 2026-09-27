package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func TestChatService_CreateChat_Success(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	chat, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b", encryptedChatKeysOnly(chatKeys("user-a", "user-b")), wrappedForPublicKeysOnly(chatKeys("user-a", "user-b")))
	if err != nil {
		t.Fatalf("CreateChat() unexpected error: %v", err)
	}
	if chat.ID == "" {
		t.Error("CreateChat() returned chat with empty ID")
	}

	isMember, _ := repo.IsMember(context.Background(), chat.ID, "user-a")
	if !isMember {
		t.Error("CreateChat() requester is not a member of the created chat")
	}
}

func TestChatService_CreateChat_Idempotent(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	first, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b", encryptedChatKeysOnly(chatKeys("user-a", "user-b")), wrappedForPublicKeysOnly(chatKeys("user-a", "user-b")))
	if err != nil {
		t.Fatalf("CreateChat() unexpected error: %v", err)
	}

	second, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b", encryptedChatKeysOnly(chatKeys("user-a", "user-b")), wrappedForPublicKeysOnly(chatKeys("user-a", "user-b")))
	if err != nil {
		t.Fatalf("CreateChat() second call unexpected error: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("CreateChat() created a duplicate chat: %q != %q", first.ID, second.ID)
	}
}

func TestChatService_CreateChat_WithSelf(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	chat, err := svc.CreateChat(context.Background(), "token", "user-a", "user-a", encryptedChatKeysOnly(chatKeys("user-a")), wrappedForPublicKeysOnly(chatKeys("user-a")))
	if err != nil {
		t.Fatalf("CreateChat() with self unexpected error: %v", err)
	}
	if chat.ChatType != domain.ChatTypePrivate {
		t.Errorf("CreateChat() with self ChatType = %q, want %q", chat.ChatType, domain.ChatTypePrivate)
	}

	members, err := repo.ListMembers(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("ListMembers() unexpected error: %v", err)
	}
	if len(members) != 1 || members[0].UserID != "user-a" {
		t.Errorf("ListMembers() = %+v, want a single member user-a", members)
	}
}

func TestChatService_CreateChat_TargetNotFound(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.CreateChat(context.Background(), "token", "user-a", "missing-user", encryptedChatKeysOnly(chatKeys("user-a", "missing-user")), wrappedForPublicKeysOnly(chatKeys("user-a", "missing-user")))
	if !errors.Is(err, domain.ErrTargetUserNotFound) {
		t.Errorf("CreateChat() error = %v, want %v", err, domain.ErrTargetUserNotFound)
	}
}

func TestChatService_ListMembers_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	userIDs, err := svc.ListMembers(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("ListMembers() unexpected error: %v", err)
	}
	if len(userIDs) != 2 {
		t.Fatalf("ListMembers() returned %d members, want 2", len(userIDs))
	}
}
