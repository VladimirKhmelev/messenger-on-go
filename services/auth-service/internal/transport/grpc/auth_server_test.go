package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestVisibleEmail_OnlyForTheCallerThemselves(t *testing.T) {
	user := &domain.User{ID: "u1", Email: "u1@example.com"}

	self := context.WithValue(context.Background(), userIDContextKey, "u1")
	if got := visibleEmail(self, user); got != "u1@example.com" {
		t.Errorf("own profile: email = %q, want it shown", got)
	}

	other := context.WithValue(context.Background(), userIDContextKey, "u2")
	if got := visibleEmail(other, user); got != "" {
		t.Errorf("someone else's profile: email = %q, want hidden", got)
	}

	if got := visibleEmail(context.Background(), user); got != "" {
		t.Errorf("no caller: email = %q, want hidden", got)
	}
}

func TestToGRPCError_InvalidOAuthCodeIsUnauthenticated(t *testing.T) {
	if got := status.Code(toGRPCError(domain.ErrInvalidOAuthCode)); got != codes.Unauthenticated {
		t.Errorf("code = %v, want %v", got, codes.Unauthenticated)
	}
}
