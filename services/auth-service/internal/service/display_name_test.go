package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestAuthService_UpdateDisplayName_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Old Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if err := svc.UpdateDisplayName(context.Background(), user.ID, "🔥 New Name 🔥"); err != nil {
		t.Fatalf("UpdateDisplayName() unexpected error: %v", err)
	}

	updated, err := svc.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() unexpected error: %v", err)
	}
	if updated.DisplayName != "🔥 New Name 🔥" {
		t.Errorf("UpdateDisplayName() display name = %q, want %q", updated.DisplayName, "🔥 New Name 🔥")
	}
}

func TestAuthService_UpdateDisplayName_TrimsWhitespace(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Old Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if err := svc.UpdateDisplayName(context.Background(), user.ID, "  Padded Name  "); err != nil {
		t.Fatalf("UpdateDisplayName() unexpected error: %v", err)
	}

	updated, err := svc.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() unexpected error: %v", err)
	}
	if updated.DisplayName != "Padded Name" {
		t.Errorf("UpdateDisplayName() display name = %q, want %q", updated.DisplayName, "Padded Name")
	}
}

func TestAuthService_UpdateDisplayName_Invalid(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Old Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err = svc.UpdateDisplayName(context.Background(), user.ID, "   ")
	if !errors.Is(err, domain.ErrInvalidDisplayName) {
		t.Errorf("UpdateDisplayName() error = %v, want %v", err, domain.ErrInvalidDisplayName)
	}
}

func TestAuthService_UpdateDisplayName_NotUniqueAcrossUsers(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.Register(context.Background(), "a@example.com", "alice", "Same Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	bob, err := svc.Register(context.Background(), "b@example.com", "bob", "Bob", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	// Unlike tag, display names are allowed to collide.
	if err := svc.UpdateDisplayName(context.Background(), bob.ID, "Same Name"); err != nil {
		t.Errorf("UpdateDisplayName() unexpected error for a duplicate display name: %v", err)
	}
}
