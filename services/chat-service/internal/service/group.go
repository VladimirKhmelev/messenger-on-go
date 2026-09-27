package service

import (
	"context"
	"log"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/google/uuid"
)

func (s *ChatService) CreateGroupChat(ctx context.Context, bearerToken, requesterID, name string, targetUserIDs []string, encryptedChatKeyByUserID, wrappedForPublicKeyByUserID map[string]string) (*domain.Chat, error) {
	if err := validateGroupName(name); err != nil {
		return nil, err
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

	chatKeyByUserID := make(map[string]domain.MemberChatKey, len(allMemberIDs))
	for _, memberID := range allMemberIDs {
		encryptedKey := encryptedChatKeyByUserID[memberID]
		wrappedKey := wrappedForPublicKeyByUserID[memberID]
		if encryptedKey == "" || wrappedKey == "" {
			return nil, domain.ErrMissingChatKey
		}
		chatKeyByUserID[memberID] = domain.MemberChatKey{EncryptedChatKey: encryptedKey, WrappedForPublicKey: wrappedKey}
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

	return s.chats.AddMember(ctx, chatID, newMemberID, domain.MemberChatKey{
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
	if err := validateMemberRole(role); err != nil {
		return err
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

func (s *ChatService) DeleteGroupChat(ctx context.Context, chatID, requesterID string) error {
	chat, err := s.chats.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.ChatType != domain.ChatTypeGroup {
		return domain.ErrNotGroupChat
	}
	if !isCreator(chat, requesterID) {
		return domain.ErrOnlyCreatorCanDeleteChat
	}

	members, err := s.chats.ListMembers(ctx, chatID)
	if err != nil {
		return err
	}
	memberUserIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberUserIDs = append(memberUserIDs, m.UserID)
	}

	if err := s.chats.DeleteChat(ctx, chatID); err != nil {
		return err
	}

	if err := s.events.PublishChatDeleted(ctx, domain.ChatDeleted{
		ChatID:        chatID,
		MemberUserIDs: memberUserIDs,
		DeletedAt:     time.Now(),
	}); err != nil {
		log.Printf("chat-service: failed to publish chat.deleted event for %s: %v", chatID, err)
	}

	return nil
}

func isCreator(chat *domain.Chat, userID string) bool {
	return chat.CreatedBy != nil && *chat.CreatedBy == userID
}

func (s *ChatService) UploadGroupAvatar(ctx context.Context, chatID, requesterID string, data []byte) error {
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

	contentType, err := validateGroupAvatar(data)
	if err != nil {
		return err
	}

	return s.chats.UpsertChatAvatar(ctx, &domain.ChatAvatar{
		ChatID:      chatID,
		Data:        data,
		ContentType: contentType,
		UpdatedAt:   time.Now(),
	})
}

func (s *ChatService) GetGroupAvatar(ctx context.Context, chatID string) (*domain.ChatAvatar, error) {
	return s.chats.GetChatAvatar(ctx, chatID)
}
