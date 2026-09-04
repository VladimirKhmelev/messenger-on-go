package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
)

const (
	HistoryDefaultLimit = 50

	MaxMessageBodyBytes = 64 * 1024
)

type AuthClient interface {
	UserExists(ctx context.Context, bearerToken, userID string) (bool, error)
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event events.MessageCreated) error
	PublishMessageUpdated(ctx context.Context, event events.MessageUpdated) error
	PublishMessageRead(ctx context.Context, event events.MessageRead) error
}

type PresenceChecker interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
	LastSeen(ctx context.Context, userID string) (int64, error)
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	SetTyping(ctx context.Context, chatID, userID string) error
	IsTyping(ctx context.Context, chatID, userID string) (bool, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, userID string) (bool, error)
}

type ChatService struct {
	chats       repository.ChatRepository
	auth        AuthClient
	events      EventPublisher
	presence    PresenceChecker
	sendLimiter RateLimiter
}

func NewChatService(chats repository.ChatRepository, auth AuthClient, eventPublisher EventPublisher, presence PresenceChecker, sendLimiter RateLimiter) *ChatService {
	return &ChatService{chats: chats, auth: auth, events: eventPublisher, presence: presence, sendLimiter: sendLimiter}
}

func (s *ChatService) CreateChat(ctx context.Context, bearerToken, requesterID, targetID string, encryptedChatKeyByUserID, wrappedForPublicKeyByUserID map[string]string) (*domain.Chat, error) {
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

	if encryptedChatKeyByUserID[requesterID] == "" || encryptedChatKeyByUserID[targetID] == "" ||
		wrappedForPublicKeyByUserID[requesterID] == "" || wrappedForPublicKeyByUserID[targetID] == "" {
		return nil, domain.ErrMissingChatKey
	}

	chat := &domain.Chat{
		ID:        uuid.NewString(),
		CreatedAt: time.Now(),
		ChatType:  domain.ChatTypePrivate,
	}

	chatKeyByUserID := map[string]repository.MemberChatKey{
		requesterID: {EncryptedChatKey: encryptedChatKeyByUserID[requesterID], WrappedForPublicKey: wrappedForPublicKeyByUserID[requesterID]},
		targetID:    {EncryptedChatKey: encryptedChatKeyByUserID[targetID], WrappedForPublicKey: wrappedForPublicKeyByUserID[targetID]},
	}

	if err := s.chats.CreateChat(ctx, chat, chatKeyByUserID); err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *ChatService) CreateGroupChat(ctx context.Context, bearerToken, requesterID, name string, targetUserIDs []string, encryptedChatKeyByUserID, wrappedForPublicKeyByUserID map[string]string) (*domain.Chat, error) {
	if name == "" {
		return nil, domain.ErrGroupNameRequired
	}
	if len(targetUserIDs) < 2 {
		return nil, domain.ErrTooFewMembers
	}

	allMemberIDs := make([]string, 0, len(targetUserIDs)+1)
	allMemberIDs = append(allMemberIDs, requesterID)
	seen := map[string]bool{requesterID: true}
	for _, targetID := range targetUserIDs {
		if targetID == requesterID || seen[targetID] {
			continue
		}
		seen[targetID] = true

		exists, err := s.auth.UserExists(ctx, bearerToken, targetID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, domain.ErrTargetUserNotFound
		}
		allMemberIDs = append(allMemberIDs, targetID)
	}

	if len(allMemberIDs) < 3 {
		return nil, domain.ErrTooFewMembers
	}
	if len(allMemberIDs) > domain.MaxGroupChatMembers {
		return nil, domain.ErrTooManyMembers
	}

	chatKeyByUserID := make(map[string]repository.MemberChatKey, len(allMemberIDs))
	for _, memberID := range allMemberIDs {
		encryptedKey := encryptedChatKeyByUserID[memberID]
		wrappedKey := wrappedForPublicKeyByUserID[memberID]
		if encryptedKey == "" || wrappedKey == "" {
			return nil, domain.ErrMissingChatKey
		}
		chatKeyByUserID[memberID] = repository.MemberChatKey{EncryptedChatKey: encryptedKey, WrappedForPublicKey: wrappedKey}
	}

	chat := &domain.Chat{
		ID:        uuid.NewString(),
		CreatedAt: time.Now(),
		ChatType:  domain.ChatTypeGroup,
		Name:      &name,
		CreatedBy: &requesterID,
	}

	if err := s.chats.CreateChat(ctx, chat, chatKeyByUserID); err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *ChatService) AddMember(ctx context.Context, bearerToken, chatID, requesterID, newMemberID, encryptedChatKey, wrappedForPublicKey string) error {
	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypeGroup {
		return domain.ErrNotGroupChat
	}

	isAdmin, err := s.chats.IsAdmin(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return domain.ErrNotChatAdmin
	}

	isMember, err := s.chats.IsMember(ctx, chatID, newMemberID)
	if err != nil {
		return err
	}
	if isMember {
		return domain.ErrAlreadyMember
	}

	count, err := s.chats.MemberCount(ctx, chatID)
	if err != nil {
		return err
	}
	if count >= domain.MaxGroupChatMembers {
		return domain.ErrTooManyMembers
	}

	exists, err := s.auth.UserExists(ctx, bearerToken, newMemberID)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrTargetUserNotFound
	}

	if encryptedChatKey == "" || wrappedForPublicKey == "" {
		return domain.ErrMissingChatKey
	}

	return s.chats.AddMember(ctx, chatID, newMemberID, repository.MemberChatKey{
		EncryptedChatKey:    encryptedChatKey,
		WrappedForPublicKey: wrappedForPublicKey,
	})
}

func (s *ChatService) RemoveMember(ctx context.Context, chatID, requesterID, targetUserID string) error {
	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypeGroup {
		return domain.ErrNotGroupChat
	}

	requesterIsAdmin, err := s.chats.IsAdmin(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !requesterIsAdmin {
		return domain.ErrNotChatAdmin
	}

	if isCreator(chat, targetUserID) {
		return domain.ErrCannotRemoveCreator
	}

	target, err := s.chats.GetMember(ctx, chatID, targetUserID)
	if err != nil {
		return err
	}
	if target.Role == domain.MemberRoleAdmin && !isCreator(chat, requesterID) {
		return domain.ErrOnlyCreatorCanManageAdmins
	}

	return s.chats.RemoveMember(ctx, chatID, targetUserID)
}

func (s *ChatService) SetMemberRole(ctx context.Context, chatID, requesterID, targetUserID string, role domain.MemberRole) error {
	if role != domain.MemberRoleAdmin && role != domain.MemberRoleMember {
		return domain.ErrInvalidRole
	}

	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypeGroup {
		return domain.ErrNotGroupChat
	}

	if !isCreator(chat, requesterID) {
		return domain.ErrOnlyCreatorCanManageAdmins
	}

	if isCreator(chat, targetUserID) {
		return domain.ErrCannotRemoveCreator
	}

	if _, err := s.chats.GetMember(ctx, chatID, targetUserID); err != nil {
		return err
	}

	return s.chats.SetRole(ctx, chatID, targetUserID, role)
}

func (s *ChatService) LeaveChat(ctx context.Context, chatID, requesterID string) error {
	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypeGroup {
		return domain.ErrNotGroupChat
	}

	if isCreator(chat, requesterID) {
		return domain.ErrCannotRemoveCreator
	}

	return s.chats.RemoveMember(ctx, chatID, requesterID)
}

func isCreator(chat *domain.Chat, userID string) bool {
	return chat.CreatedBy != nil && *chat.CreatedBy == userID
}

func (s *ChatService) GetChatKey(ctx context.Context, chatID, requesterID string) (string, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return "", err
	}
	if !isMember {
		return "", domain.ErrNotChatMember
	}

	return s.chats.GetChatKeyForUser(ctx, chatID, requesterID)
}

func (s *ChatService) ListChatKeys(ctx context.Context, chatID, requesterID string) ([]*domain.ChatMember, error) {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	return s.chats.ListMembers(ctx, chatID)
}

func (s *ChatService) UpdateChatKey(ctx context.Context, chatID, requesterID, targetUserID, encryptedChatKey, wrappedForPublicKey string) error {
	isMember, err := s.chats.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrNotChatMember
	}

	if encryptedChatKey == "" || wrappedForPublicKey == "" {
		return domain.ErrMissingChatKey
	}

	return s.chats.UpdateChatKey(ctx, chatID, targetUserID, encryptedChatKey, wrappedForPublicKey)
}

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID, body string) (*domain.Message, error) {
	if body == "" {
		return nil, domain.ErrEmptyMessage
	}
	if len(body) > MaxMessageBodyBytes {
		return nil, domain.ErrMessageTooLarge
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
	metrics.MessagesSentTotal.Inc()

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
	if newBody == "" {
		return nil, domain.ErrEmptyMessage
	}
	if len(newBody) > MaxMessageBodyBytes {
		return nil, domain.ErrMessageTooLarge
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
		isAdmin, err := s.chats.IsAdmin(ctx, message.ChatID, requesterID)
		if err != nil {
			return err
		}
		if !isAdmin {
			return domain.ErrNotMessageSender
		}
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
	if err := s.chats.MarkRead(ctx, chatID, requesterID, messageID, now); err != nil {
		return err
	}

	if err := s.events.PublishMessageRead(ctx, events.MessageRead{
		ChatID:    chatID,
		UserID:    requesterID,
		MessageID: messageID,
		ReadAt:    now,
	}); err != nil {
		log.Printf("chat-service: failed to publish msg.read event for chat %s: %v", chatID, err)
	}

	return nil
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

type ChatSummary struct {
	ChatID        string
	MemberUserIDs []string
	LastMessage   *domain.Message
	ChatType      domain.ChatType
	Name          string
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

		name := ""
		if chat.Name != nil {
			name = *chat.Name
		}

		summaries = append(summaries, &ChatSummary{
			ChatID:        chat.ID,
			MemberUserIDs: memberIDs,
			LastMessage:   lastMessage,
			ChatType:      chat.ChatType,
			Name:          name,
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

func (s *ChatService) ListChatMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error) {
	return s.chats.ListMembers(ctx, chatID)
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

func (s *ChatService) SetOnline(ctx context.Context, userID string) error {
	return s.presence.SetOnline(ctx, userID)
}

func (s *ChatService) SetOffline(ctx context.Context, userID string) error {
	return s.presence.SetOffline(ctx, userID)
}

func (s *ChatService) SetTyping(ctx context.Context, chatID, userID string) error {
	return s.presence.SetTyping(ctx, chatID, userID)
}

func (s *ChatService) GetTyping(ctx context.Context, chatID, userID string) (bool, error) {
	return s.presence.IsTyping(ctx, chatID, userID)
}
