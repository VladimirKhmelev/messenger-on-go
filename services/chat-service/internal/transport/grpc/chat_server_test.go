package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

func TestToGRPCError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"cannot chat with self", domain.ErrCannotChatWithSelf, codes.InvalidArgument},
		{"empty message", domain.ErrEmptyMessage, codes.InvalidArgument},
		{"message too large", domain.ErrMessageTooLarge, codes.InvalidArgument},
		{"missing chat key", domain.ErrMissingChatKey, codes.InvalidArgument},
		{"too few members", domain.ErrTooFewMembers, codes.InvalidArgument},
		{"too many members", domain.ErrTooManyMembers, codes.InvalidArgument},
		{"group name required", domain.ErrGroupNameRequired, codes.InvalidArgument},
		{"already member", domain.ErrAlreadyMember, codes.InvalidArgument},
		{"not group chat", domain.ErrNotGroupChat, codes.InvalidArgument},
		{"invalid role", domain.ErrInvalidRole, codes.InvalidArgument},
		{"target user not found", domain.ErrTargetUserNotFound, codes.NotFound},
		{"chat not found", domain.ErrChatNotFound, codes.NotFound},
		{"message not found", domain.ErrMessageNotFound, codes.NotFound},
		{"not chat member", domain.ErrNotChatMember, codes.PermissionDenied},
		{"not message sender", domain.ErrNotMessageSender, codes.PermissionDenied},
		{"not chat admin", domain.ErrNotChatAdmin, codes.PermissionDenied},
		{"cannot remove creator", domain.ErrCannotRemoveCreator, codes.PermissionDenied},
		{"only creator can manage admins", domain.ErrOnlyCreatorCanManageAdmins, codes.PermissionDenied},
		{"message deleted", domain.ErrMessageDeleted, codes.FailedPrecondition},
		{"message not in chat", domain.ErrMessageNotInChat, codes.InvalidArgument},
		{"too many messages", domain.ErrTooManyMessages, codes.ResourceExhausted},
		{"unknown error maps to internal", errors.New("something exploded"), codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toGRPCError(tt.err)
			if status.Code(got) != tt.want {
				t.Errorf("toGRPCError(%v) code = %v, want %v", tt.err, status.Code(got), tt.want)
			}
		})
	}
}

func TestToGRPCError_UnknownErrorHidesInternalDetails(t *testing.T) {
	err := errors.New("postgres: connection refused with credentials leaked")
	got := toGRPCError(err)

	if status.Code(got) != codes.Internal {
		t.Fatalf("toGRPCError() code = %v, want %v", status.Code(got), codes.Internal)
	}
	if got.Error() == err.Error() {
		t.Error("toGRPCError() leaked the raw internal error message to the client")
	}
}

func TestChatServer_UnauthenticatedRequests(t *testing.T) {
	s := NewChatServer(nil)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"CreateChat", func() error { _, err := s.CreateChat(ctx, &chatv1.CreateChatRequest{}); return err }},
		{"CreateGroupChat", func() error { _, err := s.CreateGroupChat(ctx, &chatv1.CreateGroupChatRequest{}); return err }},
		{"AddMember", func() error { _, err := s.AddMember(ctx, &chatv1.AddMemberRequest{}); return err }},
		{"RemoveMember", func() error { _, err := s.RemoveMember(ctx, &chatv1.RemoveMemberRequest{}); return err }},
		{"SetMemberRole", func() error { _, err := s.SetMemberRole(ctx, &chatv1.SetMemberRoleRequest{}); return err }},
		{"LeaveChat", func() error { _, err := s.LeaveChat(ctx, &chatv1.LeaveChatRequest{}); return err }},
		{"SendMessage", func() error { _, err := s.SendMessage(ctx, &chatv1.SendMessageRequest{}); return err }},
		{"GetHistory", func() error { _, err := s.GetHistory(ctx, &chatv1.GetHistoryRequest{}); return err }},
		{"ListChats", func() error { _, err := s.ListChats(ctx, &chatv1.ListChatsRequest{}); return err }},
		{"ListChatMembers", func() error { _, err := s.ListChatMembers(ctx, &chatv1.ListChatMembersRequest{}); return err }},
		{"EditMessage", func() error { _, err := s.EditMessage(ctx, &chatv1.EditMessageRequest{}); return err }},
		{"DeleteMessageForAll", func() error { _, err := s.DeleteMessageForAll(ctx, &chatv1.DeleteMessageForAllRequest{}); return err }},
		{"DeleteMessageForMe", func() error { _, err := s.DeleteMessageForMe(ctx, &chatv1.DeleteMessageForMeRequest{}); return err }},
		{"MarkRead", func() error { _, err := s.MarkRead(ctx, &chatv1.MarkReadRequest{}); return err }},
		{"GetChatKey", func() error { _, err := s.GetChatKey(ctx, &chatv1.GetChatKeyRequest{}); return err }},
		{"ListChatKeys", func() error { _, err := s.ListChatKeys(ctx, &chatv1.ListChatKeysRequest{}); return err }},
		{"UpdateChatKey", func() error { _, err := s.UpdateChatKey(ctx, &chatv1.UpdateChatKeyRequest{}); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if status.Code(err) != codes.Unauthenticated {
				t.Errorf("%s() with no user in context, code = %v, want %v", tt.name, status.Code(err), codes.Unauthenticated)
			}
		})
	}
}

func TestChatServer_Health_NoAuthRequired(t *testing.T) {
	s := NewChatServer(nil)

	resp, err := s.Health(context.Background(), &chatv1.HealthRequest{})
	if err != nil {
		t.Fatalf("Health() unexpected error: %v", err)
	}
	if !resp.GetOk() {
		t.Error("Health() Ok = false, want true")
	}
}

func TestToProtoMessage(t *testing.T) {
	now := domain.Message{
		ID: "msg-1", SenderID: "user-1", Body: "hello",
	}

	got := toProtoMessage(&now)
	if got.MessageId != "msg-1" || got.SenderUserId != "user-1" || got.Text != "hello" {
		t.Errorf("toProtoMessage() = %+v, want MessageId=msg-1 SenderUserId=user-1 Text=hello", got)
	}
	if got.Deleted {
		t.Error("toProtoMessage() Deleted = true for a message with no DeletedAt, want false")
	}
}
