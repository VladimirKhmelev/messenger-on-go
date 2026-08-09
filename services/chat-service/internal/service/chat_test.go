package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
)

type fakeChatRepository struct {
	chats    map[string]*domain.Chat
	members  map[string][]*domain.ChatMember
	messages map[string][]*domain.Message
}

func newFakeChatRepository() *fakeChatRepository {
	return &fakeChatRepository{
		chats:    make(map[string]*domain.Chat),
		members:  make(map[string][]*domain.ChatMember),
		messages: make(map[string][]*domain.Message),
	}
}

func (r *fakeChatRepository) CreateChat(_ context.Context, chat *domain.Chat, memberIDs []string) error {
	r.chats[chat.ID] = chat
	for _, id := range memberIDs {
		r.members[chat.ID] = append(r.members[chat.ID], &domain.ChatMember{
			ChatID: chat.ID, UserID: id, JoinedAt: chat.CreatedAt,
		})
	}
	return nil
}

func (r *fakeChatRepository) GetChat(_ context.Context, chatID string) (*domain.Chat, error) {
	chat, ok := r.chats[chatID]
	if !ok {
		return nil, domain.ErrChatNotFound
	}
	return chat, nil
}

func (r *fakeChatRepository) FindPrivateChat(_ context.Context, userA, userB string) (*domain.Chat, error) {
	for chatID, members := range r.members {
		if len(members) != 2 {
			continue
		}
		hasA, hasB := false, false
		for _, m := range members {
			if m.UserID == userA {
				hasA = true
			}
			if m.UserID == userB {
				hasB = true
			}
		}
		if hasA && hasB {
			return r.chats[chatID], nil
		}
	}
	return nil, domain.ErrChatNotFound
}

func (r *fakeChatRepository) IsMember(_ context.Context, chatID, userID string) (bool, error) {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeChatRepository) ListMembers(_ context.Context, chatID string) ([]*domain.ChatMember, error) {
	return r.members[chatID], nil
}

func (r *fakeChatRepository) ListChatsForUser(_ context.Context, userID string) ([]*domain.Chat, error) {
	var chats []*domain.Chat
	for chatID, members := range r.members {
		for _, m := range members {
			if m.UserID == userID {
				chats = append(chats, r.chats[chatID])
				break
			}
		}
	}
	return chats, nil
}

func (r *fakeChatRepository) CreateMessage(_ context.Context, message *domain.Message) error {
	r.messages[message.ChatID] = append(r.messages[message.ChatID], message)
	return nil
}

func (r *fakeChatRepository) ListMessages(_ context.Context, chatID string, limit int) ([]*domain.Message, error) {
	messages := r.messages[chatID]
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return messages, nil
}

func (r *fakeChatRepository) GetLastMessage(_ context.Context, chatID string) (*domain.Message, error) {
	messages := r.messages[chatID]
	if len(messages) == 0 {
		return nil, nil
	}
	return messages[len(messages)-1], nil
}

func (r *fakeChatRepository) GetMessage(_ context.Context, messageID string) (*domain.Message, error) {
	for _, messages := range r.messages {
		for _, m := range messages {
			if m.ID == messageID {
				return m, nil
			}
		}
	}
	return nil, domain.ErrMessageNotFound
}

type fakeAuthClient struct {
	existingUserIDs map[string]bool
}

func newFakeAuthClient(existingUserIDs ...string) *fakeAuthClient {
	set := make(map[string]bool, len(existingUserIDs))
	for _, id := range existingUserIDs {
		set[id] = true
	}
	return &fakeAuthClient{existingUserIDs: set}
}

func (c *fakeAuthClient) UserExists(_ context.Context, _, userID string) (bool, error) {
	return c.existingUserIDs[userID], nil
}

type fakeEventPublisher struct {
	messageCreatedEvents []events.MessageCreated
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) PublishMessageCreated(_ context.Context, event events.MessageCreated) error {
	p.messageCreatedEvents = append(p.messageCreatedEvents, event)
	return nil
}

type fakePresenceChecker struct {
	onlineUserIDs map[string]bool
}

func newFakePresenceChecker(onlineUserIDs ...string) *fakePresenceChecker {
	set := make(map[string]bool, len(onlineUserIDs))
	for _, id := range onlineUserIDs {
		set[id] = true
	}
	return &fakePresenceChecker{onlineUserIDs: set}
}

func (c *fakePresenceChecker) IsOnline(_ context.Context, userID string) (bool, error) {
	return c.onlineUserIDs[userID], nil
}

func TestChatService_CreateChat_Success(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker())

	chat, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b")
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
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker())

	first, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b")
	if err != nil {
		t.Fatalf("CreateChat() unexpected error: %v", err)
	}

	second, err := svc.CreateChat(context.Background(), "token", "user-a", "user-b")
	if err != nil {
		t.Fatalf("CreateChat() second call unexpected error: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("CreateChat() created a duplicate chat: %q != %q", first.ID, second.ID)
	}
}

func TestChatService_CreateChat_WithSelf(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.CreateChat(context.Background(), "token", "user-a", "user-a")
	if !errors.Is(err, domain.ErrCannotChatWithSelf) {
		t.Errorf("CreateChat() error = %v, want %v", err, domain.ErrCannotChatWithSelf)
	}
}

func TestChatService_CreateChat_TargetNotFound(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.CreateChat(context.Background(), "token", "user-a", "missing-user")
	if !errors.Is(err, domain.ErrTargetUserNotFound) {
		t.Errorf("CreateChat() error = %v, want %v", err, domain.ErrTargetUserNotFound)
	}
}

func TestChatService_SendMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

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
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-stranger", "hello")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_SendMessage_Empty(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "")
	if !errors.Is(err, domain.ErrEmptyMessage) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrEmptyMessage)
	}
}

func TestChatService_GetHistory_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	if _, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hi"); err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	messages, err := svc.GetHistory(context.Background(), chat.ID, "user-b", 0)
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
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.GetHistory(context.Background(), chat.ID, "user-stranger", 0)
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("GetHistory() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_ListMembers_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	userIDs, err := svc.ListMembers(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("ListMembers() unexpected error: %v", err)
	}
	if len(userIDs) != 2 {
		t.Fatalf("ListMembers() returned %d members, want 2", len(userIDs))
	}
}

func TestChatService_GetMessage_NotFound(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.GetMessage(context.Background(), "missing-message")
	if !errors.Is(err, domain.ErrMessageNotFound) {
		t.Errorf("GetMessage() error = %v, want %v", err, domain.ErrMessageNotFound)
	}
}

func TestChatService_IsOnline(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker("user-a"))

	online, err := svc.IsOnline(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if !online {
		t.Error("IsOnline() = false for an online user, want true")
	}

	online, err = svc.IsOnline(context.Background(), "user-b")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if online {
		t.Error("IsOnline() = true for an offline user, want false")
	}
}
