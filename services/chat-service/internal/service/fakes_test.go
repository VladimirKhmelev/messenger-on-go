package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

type fakeChatRepository struct {
	chats    map[string]*domain.Chat
	members  map[string][]*domain.ChatMember
	messages map[string][]*domain.Message
	events   map[string][]*domain.MessageEvent
	hidden   map[string]map[string]bool
	avatars  map[string]*domain.ChatAvatar
	blocked  map[string]map[string]bool
	reports  map[string]map[string]*domain.MessageReport
}

func newFakeChatRepository() *fakeChatRepository {
	return &fakeChatRepository{
		chats:    make(map[string]*domain.Chat),
		members:  make(map[string][]*domain.ChatMember),
		messages: make(map[string][]*domain.Message),
		events:   make(map[string][]*domain.MessageEvent),
		hidden:   make(map[string]map[string]bool),
		avatars:  make(map[string]*domain.ChatAvatar),
		blocked:  make(map[string]map[string]bool),
		reports:  make(map[string]map[string]*domain.MessageReport),
	}
}

func chatKeys(ids ...string) map[string]domain.MemberChatKey {
	keys := make(map[string]domain.MemberChatKey, len(ids))
	for _, id := range ids {
		keys[id] = domain.MemberChatKey{
			EncryptedChatKey:    "encrypted-key-" + id,
			WrappedForPublicKey: "public-key-" + id,
		}
	}
	return keys
}

func encryptedChatKeysOnly(keys map[string]domain.MemberChatKey) map[string]string {
	out := make(map[string]string, len(keys))
	for id, k := range keys {
		out[id] = k.EncryptedChatKey
	}
	return out
}

func wrappedForPublicKeysOnly(keys map[string]domain.MemberChatKey) map[string]string {
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

func (r *fakeChatRepository) CreateChat(_ context.Context, chat *domain.Chat, chatKeyByUserID map[string]domain.MemberChatKey) error {
	r.chats[chat.ID] = chat
	for id, key := range chatKeyByUserID {
		role := domain.MemberRoleMember
		if chat.CreatedBy != nil && id == *chat.CreatedBy {
			role = domain.MemberRoleAdmin
		}
		r.members[chat.ID] = append(r.members[chat.ID], &domain.ChatMember{
			ChatID: chat.ID, UserID: id, JoinedAt: chat.CreatedAt,
			EncryptedChatKey: key.EncryptedChatKey, WrappedForPublicKey: key.WrappedForPublicKey,
			Role: role,
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

func (r *fakeChatRepository) DeleteChat(_ context.Context, chatID string) error {
	delete(r.chats, chatID)
	delete(r.members, chatID)
	delete(r.messages, chatID)
	delete(r.events, chatID)
	delete(r.avatars, chatID)
	return nil
}

func (r *fakeChatRepository) FindPrivateChat(_ context.Context, userA, userB string) (*domain.Chat, error) {
	wantMemberCount := 2
	if userA == userB {
		wantMemberCount = 1
	}

	for chatID, members := range r.members {
		if len(members) != wantMemberCount || r.chats[chatID].ChatType == domain.ChatTypeGroup {
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

func (r *fakeChatRepository) IsAdmin(_ context.Context, chatID, userID string) (bool, error) {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			return m.Role == domain.MemberRoleAdmin, nil
		}
	}
	return false, nil
}

func (r *fakeChatRepository) GetMember(_ context.Context, chatID, userID string) (*domain.ChatMember, error) {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			return m, nil
		}
	}
	return nil, domain.ErrNotChatMember
}

func (r *fakeChatRepository) MemberCount(_ context.Context, chatID string) (int, error) {
	return len(r.members[chatID]), nil
}

func (r *fakeChatRepository) AddMember(_ context.Context, chatID, userID string, key domain.MemberChatKey) error {
	r.members[chatID] = append(r.members[chatID], &domain.ChatMember{
		ChatID: chatID, UserID: userID, JoinedAt: time.Now(),
		EncryptedChatKey: key.EncryptedChatKey, WrappedForPublicKey: key.WrappedForPublicKey,
		Role: domain.MemberRoleMember,
	})
	return nil
}

func (r *fakeChatRepository) RemoveMember(_ context.Context, chatID, userID string) error {
	members := r.members[chatID]
	for i, m := range members {
		if m.UserID == userID {
			r.members[chatID] = append(members[:i], members[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotChatMember
}

func (r *fakeChatRepository) SetRole(_ context.Context, chatID, userID string, role domain.MemberRole) error {
	for _, m := range r.members[chatID] {
		if m.UserID == userID {
			m.Role = role
			return nil
		}
	}
	return domain.ErrNotChatMember
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

func (r *fakeChatRepository) UpsertChatAvatar(_ context.Context, avatar *domain.ChatAvatar) error {
	r.avatars[avatar.ChatID] = avatar
	return nil
}

func (r *fakeChatRepository) GetChatAvatar(_ context.Context, chatID string) (*domain.ChatAvatar, error) {
	avatar, ok := r.avatars[chatID]
	if !ok {
		return nil, domain.ErrGroupAvatarNotFound
	}
	return avatar, nil
}

func (r *fakeChatRepository) BlockUser(_ context.Context, blockerID, blockedID string) error {
	if r.blocked[blockerID] == nil {
		r.blocked[blockerID] = make(map[string]bool)
	}
	r.blocked[blockerID][blockedID] = true
	return nil
}

func (r *fakeChatRepository) UnblockUser(_ context.Context, blockerID, blockedID string) error {
	delete(r.blocked[blockerID], blockedID)
	return nil
}

func (r *fakeChatRepository) IsBlocked(_ context.Context, userA, userB string) (bool, error) {
	return r.blocked[userA][userB] || r.blocked[userB][userA], nil
}

func (r *fakeChatRepository) ListBlockedUsers(_ context.Context, blockerID string) ([]*domain.BlockedUser, error) {
	var result []*domain.BlockedUser
	for blockedID := range r.blocked[blockerID] {
		result = append(result, &domain.BlockedUser{BlockerID: blockerID, BlockedID: blockedID})
	}
	return result, nil
}

func (r *fakeChatRepository) CreateMessageReport(_ context.Context, report *domain.MessageReport) error {
	if r.reports[report.MessageID] == nil {
		r.reports[report.MessageID] = make(map[string]*domain.MessageReport)
	}
	r.reports[report.MessageID][report.ReporterID] = report
	return nil
}

func (r *fakeChatRepository) HasReported(_ context.Context, messageID, reporterID string) (bool, error) {
	_, ok := r.reports[messageID][reporterID]
	return ok, nil
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
	messageCreatedEvents []domain.MessageCreated
	messageUpdatedEvents []domain.MessageUpdated
	messageReadEvents    []domain.MessageRead
	chatDeletedEvents    []domain.ChatDeleted
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) PublishMessageCreated(_ context.Context, event domain.MessageCreated) error {
	p.messageCreatedEvents = append(p.messageCreatedEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishMessageUpdated(_ context.Context, event domain.MessageUpdated) error {
	p.messageUpdatedEvents = append(p.messageUpdatedEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishMessageRead(_ context.Context, event domain.MessageRead) error {
	p.messageReadEvents = append(p.messageReadEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishChatDeleted(_ context.Context, event domain.ChatDeleted) error {
	p.chatDeletedEvents = append(p.chatDeletedEvents, event)
	return nil
}

type fakePresenceChecker struct {
	onlineUserIDs map[string]bool
	lastSeen      map[string]int64
	typing        map[string]bool
}

func newFakePresenceChecker(onlineUserIDs ...string) *fakePresenceChecker {
	set := make(map[string]bool, len(onlineUserIDs))
	for _, id := range onlineUserIDs {
		set[id] = true
	}
	return &fakePresenceChecker{onlineUserIDs: set, lastSeen: make(map[string]int64), typing: make(map[string]bool)}
}

func (c *fakePresenceChecker) IsOnline(_ context.Context, userID string) (bool, error) {
	return c.onlineUserIDs[userID], nil
}

func (c *fakePresenceChecker) LastSeen(_ context.Context, userID string) (int64, error) {
	return c.lastSeen[userID], nil
}

func (c *fakePresenceChecker) SetOnline(_ context.Context, userID string) error {
	c.onlineUserIDs[userID] = true
	return nil
}

func (c *fakePresenceChecker) SetOffline(_ context.Context, userID string) error {
	delete(c.onlineUserIDs, userID)
	return nil
}

func (c *fakePresenceChecker) SetTyping(_ context.Context, chatID, userID string) error {
	c.typing[chatID+":"+userID] = true
	return nil
}

func (c *fakePresenceChecker) IsTyping(_ context.Context, chatID, userID string) (bool, error) {
	return c.typing[chatID+":"+userID], nil
}

type fakeRateLimiter struct {
	allow bool
}

func newFakeRateLimiter() *fakeRateLimiter {
	return &fakeRateLimiter{allow: true}
}

func (l *fakeRateLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return l.allow, nil
}

func newFakeGroupChat(repo *fakeChatRepository, creatorID string, memberIDs ...string) *domain.Chat {
	name := "Test Group"
	chat := &domain.Chat{
		ID: uuid.NewString(), CreatedAt: time.Now(),
		ChatType: domain.ChatTypeGroup, Name: &name, CreatedBy: &creatorID,
	}
	allIDs := append([]string{creatorID}, memberIDs...)
	_ = repo.CreateChat(context.Background(), chat, chatKeys(allIDs...))
	return chat
}

var pngMagicBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
