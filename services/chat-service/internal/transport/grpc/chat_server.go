package grpc

import (
	"context"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/service"
)

type ChatServer struct {
	chatv1.UnimplementedChatServiceServer

	chat *service.ChatService
}

func NewChatServer(chat *service.ChatService) *ChatServer {
	return &ChatServer{chat: chat}
}

func (s *ChatServer) Health(ctx context.Context, req *chatv1.HealthRequest) (*chatv1.HealthResponse, error) {
	return &chatv1.HealthResponse{Ok: true}, nil
}

func (s *ChatServer) CreateChat(ctx context.Context, req *chatv1.CreateChatRequest) (*chatv1.CreateChatResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	bearerToken, err := BearerTokenFromIncomingContext(ctx)
	if err != nil {
		return nil, err
	}

	chat, err := s.chat.CreateChat(ctx, bearerToken, requesterID, req.GetTargetUserId(), req.GetEncryptedChatKey(), req.GetWrappedForPublicKey())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.CreateChatResponse{ChatId: chat.ID}, nil
}

func (s *ChatServer) CreateGroupChat(ctx context.Context, req *chatv1.CreateGroupChatRequest) (*chatv1.CreateGroupChatResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	bearerToken, err := BearerTokenFromIncomingContext(ctx)
	if err != nil {
		return nil, err
	}

	chat, err := s.chat.CreateGroupChat(ctx, bearerToken, requesterID, req.GetName(), req.GetTargetUserIds(), req.GetEncryptedChatKey(), req.GetWrappedForPublicKey())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.CreateGroupChatResponse{ChatId: chat.ID}, nil
}

func (s *ChatServer) AddMember(ctx context.Context, req *chatv1.AddMemberRequest) (*chatv1.AddMemberResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	bearerToken, err := BearerTokenFromIncomingContext(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.chat.AddMember(ctx, bearerToken, req.GetChatId(), requesterID, req.GetUserId(), req.GetEncryptedChatKey(), req.GetWrappedForPublicKey()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.AddMemberResponse{}, nil
}

func (s *ChatServer) RemoveMember(ctx context.Context, req *chatv1.RemoveMemberRequest) (*chatv1.RemoveMemberResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.RemoveMember(ctx, req.GetChatId(), requesterID, req.GetUserId()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.RemoveMemberResponse{}, nil
}

func (s *ChatServer) SetMemberRole(ctx context.Context, req *chatv1.SetMemberRoleRequest) (*chatv1.SetMemberRoleResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.SetMemberRole(ctx, req.GetChatId(), requesterID, req.GetUserId(), domain.MemberRole(req.GetRole())); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.SetMemberRoleResponse{}, nil
}

func (s *ChatServer) LeaveChat(ctx context.Context, req *chatv1.LeaveChatRequest) (*chatv1.LeaveChatResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.LeaveChat(ctx, req.GetChatId(), requesterID); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.LeaveChatResponse{}, nil
}

func (s *ChatServer) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
	senderID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	message, err := s.chat.SendMessage(ctx, req.GetChatId(), senderID, req.GetText())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.SendMessageResponse{MessageId: message.ID}, nil
}

func (s *ChatServer) GetHistory(ctx context.Context, req *chatv1.GetHistoryRequest) (*chatv1.GetHistoryResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	messages, err := s.chat.GetHistory(ctx, req.GetChatId(), requesterID, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, toGRPCError(err)
	}

	result := make([]*chatv1.Message, 0, len(messages))
	for _, m := range messages {
		result = append(result, toProtoMessage(m))
	}

	return &chatv1.GetHistoryResponse{Messages: result}, nil
}

func (s *ChatServer) ListChats(ctx context.Context, req *chatv1.ListChatsRequest) (*chatv1.ListChatsResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	summaries, err := s.chat.ListChats(ctx, requesterID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	result := make([]*chatv1.ChatSummary, 0, len(summaries))
	for _, summary := range summaries {
		var lastMessage *chatv1.Message
		if summary.LastMessage != nil {
			lastMessage = toProtoMessage(summary.LastMessage)
		}

		result = append(result, &chatv1.ChatSummary{
			ChatId:        summary.ChatID,
			MemberUserIds: summary.MemberUserIDs,
			LastMessage:   lastMessage,
			ChatType:      string(summary.ChatType),
			Name:          summary.Name,
		})
	}

	return &chatv1.ListChatsResponse{Chats: result}, nil
}

func (s *ChatServer) ListMembers(ctx context.Context, req *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error) {
	members, err := s.chat.ListChatMembers(ctx, req.GetChatId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	userIDs := make([]string, 0, len(members))
	memberInfos := make([]*chatv1.ChatMemberInfo, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
		memberInfos = append(memberInfos, &chatv1.ChatMemberInfo{UserId: m.UserID, Role: string(m.Role)})
	}

	return &chatv1.ListMembersResponse{UserIds: userIDs, Members: memberInfos}, nil
}

func (s *ChatServer) GetMessage(ctx context.Context, req *chatv1.GetMessageRequest) (*chatv1.GetMessageResponse, error) {
	message, err := s.chat.GetMessage(ctx, req.GetMessageId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetMessageResponse{Message: toProtoMessage(message)}, nil
}

func (s *ChatServer) EditMessage(ctx context.Context, req *chatv1.EditMessageRequest) (*chatv1.EditMessageResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	message, err := s.chat.EditMessage(ctx, req.GetMessageId(), requesterID, req.GetText())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.EditMessageResponse{Message: toProtoMessage(message)}, nil
}

func (s *ChatServer) DeleteMessageForAll(ctx context.Context, req *chatv1.DeleteMessageForAllRequest) (*chatv1.DeleteMessageForAllResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.DeleteMessageForAll(ctx, req.GetMessageId(), requesterID); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.DeleteMessageForAllResponse{}, nil
}

func (s *ChatServer) DeleteMessageForMe(ctx context.Context, req *chatv1.DeleteMessageForMeRequest) (*chatv1.DeleteMessageForMeResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.DeleteMessageForMe(ctx, req.GetMessageId(), requesterID); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.DeleteMessageForMeResponse{}, nil
}

func (s *ChatServer) MarkRead(ctx context.Context, req *chatv1.MarkReadRequest) (*chatv1.MarkReadResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.MarkRead(ctx, req.GetChatId(), requesterID, req.GetMessageId()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.MarkReadResponse{}, nil
}

func (s *ChatServer) GetReadStatus(ctx context.Context, req *chatv1.GetReadStatusRequest) (*chatv1.GetReadStatusResponse, error) {
	lastReadMessageID, err := s.chat.GetReadStatus(ctx, req.GetChatId(), req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetReadStatusResponse{LastReadMessageId: lastReadMessageID}, nil
}

func (s *ChatServer) GetChatKey(ctx context.Context, req *chatv1.GetChatKeyRequest) (*chatv1.GetChatKeyResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	encryptedChatKey, err := s.chat.GetChatKey(ctx, req.GetChatId(), requesterID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetChatKeyResponse{EncryptedChatKey: encryptedChatKey}, nil
}

func (s *ChatServer) ListChatKeys(ctx context.Context, req *chatv1.ListChatKeysRequest) (*chatv1.ListChatKeysResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	members, err := s.chat.ListChatKeys(ctx, req.GetChatId(), requesterID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	memberKeys := make([]*chatv1.MemberChatKey, 0, len(members))
	for _, m := range members {
		memberKeys = append(memberKeys, &chatv1.MemberChatKey{
			UserId:              m.UserID,
			EncryptedChatKey:    m.EncryptedChatKey,
			WrappedForPublicKey: m.WrappedForPublicKey,
		})
	}

	return &chatv1.ListChatKeysResponse{MemberKeys: memberKeys}, nil
}

func (s *ChatServer) UpdateChatKey(ctx context.Context, req *chatv1.UpdateChatKeyRequest) (*chatv1.UpdateChatKeyResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.chat.UpdateChatKey(ctx, req.GetChatId(), requesterID, req.GetUserId(), req.GetEncryptedChatKey(), req.GetWrappedForPublicKey()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.UpdateChatKeyResponse{}, nil
}

func toProtoMessage(m *domain.Message) *chatv1.Message {
	result := &chatv1.Message{
		MessageId:     m.ID,
		SenderUserId:  m.SenderID,
		Text:          m.Body,
		CreatedAtUnix: m.CreatedAt.Unix(),
		Deleted:       m.DeletedAt != nil,
	}
	if m.EditedAt != nil {
		result.EditedAtUnix = m.EditedAt.Unix()
	}
	return result
}

func (s *ChatServer) GetPresence(ctx context.Context, req *chatv1.GetPresenceRequest) (*chatv1.GetPresenceResponse, error) {
	online, lastSeenUnix, err := s.chat.GetPresence(ctx, req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetPresenceResponse{Online: online, LastSeenUnix: lastSeenUnix}, nil
}

func (s *ChatServer) SetOnline(ctx context.Context, req *chatv1.SetOnlineRequest) (*chatv1.SetOnlineResponse, error) {
	if err := s.chat.SetOnline(ctx, req.GetUserId()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.SetOnlineResponse{}, nil
}

func (s *ChatServer) SetOffline(ctx context.Context, req *chatv1.SetOfflineRequest) (*chatv1.SetOfflineResponse, error) {
	if err := s.chat.SetOffline(ctx, req.GetUserId()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.SetOfflineResponse{}, nil
}

func (s *ChatServer) ListContacts(ctx context.Context, req *chatv1.ListContactsRequest) (*chatv1.ListContactsResponse, error) {
	userIDs, err := s.chat.ListContacts(ctx, req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.ListContactsResponse{UserIds: userIDs}, nil
}

func (s *ChatServer) SetTyping(ctx context.Context, req *chatv1.SetTypingRequest) (*chatv1.SetTypingResponse, error) {
	if err := s.chat.SetTyping(ctx, req.GetChatId(), req.GetUserId()); err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.SetTypingResponse{}, nil
}

func (s *ChatServer) GetTyping(ctx context.Context, req *chatv1.GetTypingRequest) (*chatv1.GetTypingResponse, error) {
	typing, err := s.chat.GetTyping(ctx, req.GetChatId(), req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetTypingResponse{Typing: typing}, nil
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrCannotChatWithSelf),
		errors.Is(err, domain.ErrEmptyMessage),
		errors.Is(err, domain.ErrMessageTooLarge),
		errors.Is(err, domain.ErrMissingChatKey),
		errors.Is(err, domain.ErrTooFewMembers),
		errors.Is(err, domain.ErrTooManyMembers),
		errors.Is(err, domain.ErrGroupNameRequired),
		errors.Is(err, domain.ErrAlreadyMember),
		errors.Is(err, domain.ErrNotGroupChat),
		errors.Is(err, domain.ErrInvalidRole):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrTargetUserNotFound),
		errors.Is(err, domain.ErrChatNotFound),
		errors.Is(err, domain.ErrMessageNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrNotChatMember),
		errors.Is(err, domain.ErrNotMessageSender),
		errors.Is(err, domain.ErrNotChatAdmin),
		errors.Is(err, domain.ErrCannotRemoveCreator),
		errors.Is(err, domain.ErrOnlyCreatorCanManageAdmins):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrMessageDeleted):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrMessageNotInChat):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrTooManyMessages):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		log.Printf("chat-service: internal error: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
