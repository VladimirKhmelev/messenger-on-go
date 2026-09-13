package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mediav1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/media/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

func TestToGRPCError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"empty content type", domain.ErrEmptyContentType, codes.InvalidArgument},
		{"invalid size", domain.ErrInvalidSize, codes.InvalidArgument},
		{"size too large", domain.ErrSizeTooLarge, codes.InvalidArgument},
		{"content type blocked", domain.ErrContentTypeBlocked, codes.InvalidArgument},
		{"media not found", domain.ErrMediaNotFound, codes.NotFound},
		{"not chat member", domain.ErrNotChatMember, codes.PermissionDenied},
		{"upload not confirmed", domain.ErrUploadNotConfirmed, codes.FailedPrecondition},
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

func TestMediaServer_UnauthenticatedRequests(t *testing.T) {
	s := NewMediaServer(nil)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"RequestUpload", func() error { _, err := s.RequestUpload(ctx, &mediav1.RequestUploadRequest{}); return err }},
		{"ConfirmUpload", func() error { _, err := s.ConfirmUpload(ctx, &mediav1.ConfirmUploadRequest{}); return err }},
		{"GetDownloadURL", func() error { _, err := s.GetDownloadURL(ctx, &mediav1.GetDownloadURLRequest{}); return err }},
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

func TestMediaServer_Health_NoAuthRequired(t *testing.T) {
	s := NewMediaServer(nil)

	resp, err := s.Health(context.Background(), &mediav1.HealthRequest{})
	if err != nil {
		t.Fatalf("Health() unexpected error: %v", err)
	}
	if !resp.GetOk() {
		t.Error("Health() Ok = false, want true")
	}
}
