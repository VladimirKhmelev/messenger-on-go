package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

func TestChatService_UploadGroupAvatar_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := newFakeGroupChat(repo, "user-a", "user-b")
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	err := svc.UploadGroupAvatar(context.Background(), chat.ID, "user-a", pngMagicBytes)
	if err != nil {
		t.Fatalf("UploadGroupAvatar() unexpected error: %v", err)
	}

	avatar, err := svc.GetGroupAvatar(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("GetGroupAvatar() unexpected error: %v", err)
	}
	if avatar.ContentType != "image/png" {
		t.Errorf("GetGroupAvatar() ContentType = %q, want image/png", avatar.ContentType)
	}
}

func TestChatService_DeleteGroupChat_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := newFakeGroupChat(repo, "user-a", "user-b", "user-c")
	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker(), newFakeRateLimiter())

	if err := svc.DeleteGroupChat(context.Background(), chat.ID, "user-a"); err != nil {
		t.Fatalf("DeleteGroupChat() unexpected error: %v", err)
	}

	if _, err := repo.GetChat(context.Background(), chat.ID); !errors.Is(err, domain.ErrChatNotFound) {
		t.Errorf("GetChat() after delete error = %v, want %v", err, domain.ErrChatNotFound)
	}

	members, err := repo.ListMembers(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("ListMembers() unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("ListMembers() after delete = %+v, want empty", members)
	}

	if len(publisher.chatDeletedEvents) != 1 {
		t.Fatalf("PublishChatDeleted() called %d times, want 1", len(publisher.chatDeletedEvents))
	}
	event := publisher.chatDeletedEvents[0]
	if event.ChatID != chat.ID {
		t.Errorf("ChatDeleted event ChatID = %q, want %q", event.ChatID, chat.ID)
	}
	gotMembers := map[string]bool{}
	for _, id := range event.MemberUserIDs {
		gotMembers[id] = true
	}
	for _, want := range []string{"user-a", "user-b", "user-c"} {
		if !gotMembers[want] {
			t.Errorf("ChatDeleted event MemberUserIDs = %v, missing %q", event.MemberUserIDs, want)
		}
	}
}

func TestChatService_CreateGroupChat_EmptyName(t *testing.T) {
	svc := NewChatService(newFakeChatRepository(), newFakeAuthClient("user-b", "user-c"), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	keys := chatKeys("user-a", "user-b", "user-c")
	_, err := svc.CreateGroupChat(context.Background(), "token", "user-a", "", []string{"user-b", "user-c"},
		encryptedChatKeysOnly(keys), wrappedForPublicKeysOnly(keys))
	if !errors.Is(err, domain.ErrGroupNameRequired) {
		t.Errorf("CreateGroupChat() error = %v, want %v", err, domain.ErrGroupNameRequired)
	}
}

func TestChatService_SetMemberRole_InvalidRole(t *testing.T) {
	repo := newFakeChatRepository()
	chat := newFakeGroupChat(repo, "user-a", "user-b", "user-c")
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	err := svc.SetMemberRole(context.Background(), chat.ID, "user-a", "user-b", "owner")
	if !errors.Is(err, domain.ErrInvalidRole) {
		t.Errorf("SetMemberRole() error = %v, want %v", err, domain.ErrInvalidRole)
	}
}

func TestChatService_UploadGroupAvatar_InvalidType(t *testing.T) {
	repo := newFakeChatRepository()
	chat := newFakeGroupChat(repo, "user-a", "user-b")
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker(), newFakeRateLimiter())

	err := svc.UploadGroupAvatar(context.Background(), chat.ID, "user-a", []byte("not an image"))
	if !errors.Is(err, domain.ErrInvalidGroupAvatarType) {
		t.Errorf("UploadGroupAvatar() error = %v, want %v", err, domain.ErrInvalidGroupAvatarType)
	}
}
