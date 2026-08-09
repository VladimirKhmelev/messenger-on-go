package grpc

import (
	"context"
	"errors"

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

	chat, err := s.chat.CreateChat(ctx, bearerToken, requesterID, req.GetTargetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.CreateChatResponse{ChatId: chat.ID}, nil
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

	messages, err := s.chat.GetHistory(ctx, req.GetChatId(), requesterID, int(req.GetLimit()))
	if err != nil {
		return nil, toGRPCError(err)
	}

	result := make([]*chatv1.Message, 0, len(messages))
	for _, m := range messages {
		result = append(result, &chatv1.Message{
			MessageId:     m.ID,
			SenderUserId:  m.SenderID,
			Text:          m.Body,
			CreatedAtUnix: m.CreatedAt.Unix(),
		})
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
			lastMessage = &chatv1.Message{
				MessageId:     summary.LastMessage.ID,
				SenderUserId:  summary.LastMessage.SenderID,
				Text:          summary.LastMessage.Body,
				CreatedAtUnix: summary.LastMessage.CreatedAt.Unix(),
			}
		}

		result = append(result, &chatv1.ChatSummary{
			ChatId:        summary.ChatID,
			MemberUserIds: summary.MemberUserIDs,
			LastMessage:   lastMessage,
		})
	}

	return &chatv1.ListChatsResponse{Chats: result}, nil
}

func (s *ChatServer) ListMembers(ctx context.Context, req *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error) {
	userIDs, err := s.chat.ListMembers(ctx, req.GetChatId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.ListMembersResponse{UserIds: userIDs}, nil
}

func (s *ChatServer) GetMessage(ctx context.Context, req *chatv1.GetMessageRequest) (*chatv1.GetMessageResponse, error) {
	message, err := s.chat.GetMessage(ctx, req.GetMessageId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetMessageResponse{
		Message: &chatv1.Message{
			MessageId:     message.ID,
			SenderUserId:  message.SenderID,
			Text:          message.Body,
			CreatedAtUnix: message.CreatedAt.Unix(),
		},
	}, nil
}

func (s *ChatServer) GetPresence(ctx context.Context, req *chatv1.GetPresenceRequest) (*chatv1.GetPresenceResponse, error) {
	online, lastSeenUnix, err := s.chat.GetPresence(ctx, req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &chatv1.GetPresenceResponse{Online: online, LastSeenUnix: lastSeenUnix}, nil
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

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrCannotChatWithSelf),
		errors.Is(err, domain.ErrEmptyMessage):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrTargetUserNotFound),
		errors.Is(err, domain.ErrChatNotFound),
		errors.Is(err, domain.ErrMessageNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrNotChatMember):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
