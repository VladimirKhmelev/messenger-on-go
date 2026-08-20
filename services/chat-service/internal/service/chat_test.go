package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
)

type fakeChatRepository struct {
	chats    map[string]*domain.Chat
	members  map[string][]*domain.ChatMember
	messages map[string][]*domain.Message
	events   map[string][]*domain.MessageEvent
	hidden   map[string]map[string]bool
}

func newFakeChatRepository() *fakeChatRepository {
	return &fakeChatRepository{
		chats:    make(map[string]*domain.Chat),
		members:  make(map[string][]*domain.ChatMember),
		messages: make(map[string][]*domain.Message),
		events:   make(map[string][]*domain.MessageEvent),
		hidden:   make(map[string]map[string]bool),
	}
}

func chatKeys(ids ...string) map[string]repository.MemberChatKey {
	keys := make(map[string]repository.MemberChatKey, len(ids))
	for _, id := range ids {
		keys[id] = repository.MemberChatKey{
			EncryptedChatKey:    "encrypted-key-" + id,
			WrappedForPublicKey: "public-key-" + id,
		}
	}
	return keys
}

func encryptedChatKeysOnly(keys map[string]repository.MemberChatKey) map[string]string {
	out := make(map[string]string, len(keys))
	for id, k := range keys {
		out[id] = k.EncryptedChatKey
	}
	return out
}

func wrappedForPublicKeysOnly(keys map[string]repository.MemberChatKey) map[string]string {
	out := make(map[string]string, len(keys))
	for id, k := range keys {
		out[id] = k.WrappedForPublicKey
	}
	return out
}

func (r *fakeChatRepository) project(m *domain.Message) *domain.Message {
	projected := *m
	for _, e := range r.events[m.ID] {
		switch e.Type {
		case domain.MessageEventEdited:
			projected.Body = *e.NewBody
			t := e.CreatedAt
			projected.EditedAt = &t
		case domain.MessageEventDeletedForAll:
			t := e.CreatedAt
			projected.DeletedAt = &t
		}
	}
	return &projected
}

func (r *fakeChatRepository) CreateChat(_ context.Context, chat *domain.Chat, chatKeyByUserID map[string]repository.MemberChatKey) error {
	r.chats[chat.ID] = chat
	for id, key := range chatKeyByUserID {
		r.members[chat.ID] = append(r.members[chat.ID], &domain.ChatMember{
			ChatID: chat.ID, UserID: id, JoinedAt: chat.CreatedAt,
			EncryptedChatKey: key.EncryptedChatKey, WrappedForPublicKey: key.WrappedForPublicKey,
		})
	}
	return nil
}

func (r *fakeChatRepository) UpdateChatKey(_ context.Context, chatID, userID, encryptedChatKey, wrappedForPublicKey string) error {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			m.EncryptedChatKey = encryptedChatKey
			m.WrappedForPublicKey = wrappedForPublicKey
			return nil
		}
	}
	return domain.ErrNotChatMember
}

func (r *fakeChatRepository) GetChatKeyForUser(_ context.Context, chatID, userID string) (string, error) {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			return m.EncryptedChatKey, nil
		}
	}
	return "", domain.ErrNotChatMember
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

func (r *fakeChatRepository) ListMessages(_ context.Context, chatID, requesterID string, limit, offset int) ([]*domain.Message, error) {
	var visible []*domain.Message
	for _, m := range r.messages[chatID] {
		if r.hidden[m.ID][requesterID] {
			continue
		}
		visible = append(visible, r.project(m))
	}
	if offset > 0 {
		if offset >= len(visible) {
			return nil, nil
		}
		visible = visible[:len(visible)-offset]
	}
	if len(visible) > limit {
		visible = visible[len(visible)-limit:]
	}
	return visible, nil
}

func (r *fakeChatRepository) GetLastMessage(_ context.Context, chatID, requesterID string) (*domain.Message, error) {
	messages := r.messages[chatID]
	for i := len(messages) - 1; i >= 0; i-- {
		if r.hidden[messages[i].ID][requesterID] {
			continue
		}
		return r.project(messages[i]), nil
	}
	return nil, nil
}

func (r *fakeChatRepository) GetMessage(_ context.Context, messageID string) (*domain.Message, error) {
	for _, messages := range r.messages {
		for _, m := range messages {
			if m.ID == messageID {
				return r.project(m), nil
			}
		}
	}
	return nil, domain.ErrMessageNotFound
}

func (r *fakeChatRepository) AppendMessageEvent(_ context.Context, event *domain.MessageEvent) error {
	r.events[event.MessageID] = append(r.events[event.MessageID], event)
	return nil
}

func (r *fakeChatRepository) HideMessageForUser(_ context.Context, messageID, userID string) error {
	if r.hidden[messageID] == nil {
		r.hidden[messageID] = make(map[string]bool)
	}
	r.hidden[messageID][userID] = true
	return nil
}

func (r *fakeChatRepository) MarkRead(_ context.Context, chatID, userID, messageID string, readAt time.Time) error {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			id := messageID
			t := readAt
			m.LastReadMessageID = &id
			m.LastReadAt = &t
			return nil
		}
	}
	return domain.ErrNotChatMember
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
	messageUpdatedEvents []events.MessageUpdated
	messageReadEvents    []events.MessageRead
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) PublishMessageCreated(_ context.Context, event events.MessageCreated) error {
	p.messageCreatedEvents = append(p.messageCreatedEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishMessageUpdated(_ context.Context, event events.MessageUpdated) error {
	p.messageUpdatedEvents = append(p.messageUpdatedEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishMessageRead(_ context.Context, event events.MessageRead) error {
	p.messageReadEvents = append(p.messageReadEvents, event)
	return nil
}

type fakePresenceChecker struct {
	onlineUserIDs map[string]bool
	lastSeen      map[string]int64
}

func newFakePresenceChecker(onlineUserIDs ...string) *fakePresenceChecker {
	set := make(map[string]bool, len(onlineUserIDs))
	for _, id := range onlineUserIDs {
		set[id] = true
	}
	return &fakePresenceChecker{onlineUserIDs: set, lastSeen: make(map[string]int64)}
}

func (c *fakePresenceChecker) IsOnline(_ context.Context, userID string) (bool, error) {
	return c.onlineUserIDs[userID], nil
}

func (c *fakePresenceChecker) LastSeen(_ context.Context, userID string) (int64, error) {
	return c.lastSeen[userID], nil
}

func (c *fakePresenceChecker) SetOffline(_ context.Context, userID string) error {
	delete(c.onlineUserIDs, userID)
	return nil
}

func TestChatService_CreateChat_Success(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker())

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
	svc := NewChatService(repo, newFakeAuthClient("user-a", "user-b"), newFakeEventPublisher(), newFakePresenceChecker())

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
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.CreateChat(context.Background(), "token", "user-a", "user-a", encryptedChatKeysOnly(chatKeys("user-a")), wrappedForPublicKeysOnly(chatKeys("user-a")))
	if !errors.Is(err, domain.ErrCannotChatWithSelf) {
		t.Errorf("CreateChat() error = %v, want %v", err, domain.ErrCannotChatWithSelf)
	}
}

func TestChatService_CreateChat_TargetNotFound(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient("user-a"), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.CreateChat(context.Background(), "token", "user-a", "missing-user", encryptedChatKeysOnly(chatKeys("user-a", "missing-user")), wrappedForPublicKeysOnly(chatKeys("user-a", "missing-user")))
	if !errors.Is(err, domain.ErrTargetUserNotFound) {
		t.Errorf("CreateChat() error = %v, want %v", err, domain.ErrTargetUserNotFound)
	}
}

func TestChatService_SendMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

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
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-stranger", "hello")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestChatService_SendMessage_Empty(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "")
	if !errors.Is(err, domain.ErrEmptyMessage) {
		t.Errorf("SendMessage() error = %v, want %v", err, domain.ErrEmptyMessage)
	}
}

func TestChatService_GetHistory_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

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

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

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

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	messageInB, err := svc.SendMessage(context.Background(), chatB.ID, "user-a", "hi")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.MarkRead(context.Background(), chatA.ID, "user-b", messageInB.ID)
	if !errors.Is(err, domain.ErrMessageNotInChat) {
		t.Errorf("MarkRead() error = %v, want %v", err, domain.ErrMessageNotInChat)
	}
}

func TestChatService_ListMembers_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

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

func TestChatService_GetPresence(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker("user-a"))

	online, _, err := svc.GetPresence(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if !online {
		t.Error("GetPresence() online = false for an online user, want true")
	}

	online, _, err = svc.GetPresence(context.Background(), "user-b")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if online {
		t.Error("GetPresence() online = true for an offline user, want false")
	}
}

func TestChatService_SetOffline(t *testing.T) {
	repo := newFakeChatRepository()
	presence := newFakePresenceChecker("user-a")
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), presence)

	if err := svc.SetOffline(context.Background(), "user-a"); err != nil {
		t.Fatalf("SetOffline() unexpected error: %v", err)
	}

	online, _, err := svc.GetPresence(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if online {
		t.Error("GetPresence() online = true after SetOffline, want false")
	}
}

func TestChatService_EditMessage_Success(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker())

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

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

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
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker())

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

	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker())

	sent, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	err = svc.DeleteMessageForMe(context.Background(), sent.ID, "user-stranger")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("DeleteMessageForMe() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}
