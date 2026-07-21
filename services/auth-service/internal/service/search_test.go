package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestAuthService_GetUserByTag_Success(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "abcd1234")
	repo.usersByTag[user.Tag] = user
	svc := newTestAuthService(repo)

	got, err := svc.GetUserByTag(context.Background(), user.Tag)
	if err != nil {
		t.Fatalf("GetUserByTag() unexpected error: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("GetUserByTag() returned ID = %q, want %q", got.ID, user.ID)
	}
}

func TestAuthService_GetUserByTag_NotFound(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.GetUserByTag(context.Background(), "missing_tag")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("GetUserByTag() error = %v, want %v", err, domain.ErrUserNotFound)
	}
}

func TestAuthService_SearchUsers_Success(t *testing.T) {
	repo := newFakeUserRepository()
	for _, tag := range []string{"john_doe", "john_smith", "jane_doe"} {
		user := newTestUser(tag+"@example.com", "abcd1234")
		user.Tag = tag
		repo.usersByTag[tag] = user
	}
	svc := newTestAuthService(repo)

	got, err := svc.SearchUsers(context.Background(), "john")
	if err != nil {
		t.Fatalf("SearchUsers() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SearchUsers() returned %d users, want 2", len(got))
	}
}

func TestAuthService_SearchUsers_QueryTooShort(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.SearchUsers(context.Background(), "jo")
	if !errors.Is(err, domain.ErrSearchQueryTooShort) {
		t.Errorf("SearchUsers() error = %v, want %v", err, domain.ErrSearchQueryTooShort)
	}
}

func TestAuthService_SearchUsers_LimitApplied(t *testing.T) {
	repo := newFakeUserRepository()
	for i := 0; i < SearchUsersLimit+5; i++ {
		tag := "user_" + string(rune('a'+i))
		user := newTestUser(tag+"@example.com", "abcd1234")
		user.Tag = tag
		repo.usersByTag[tag] = user
	}
	svc := newTestAuthService(repo)

	got, err := svc.SearchUsers(context.Background(), "user")
	if err != nil {
		t.Fatalf("SearchUsers() unexpected error: %v", err)
	}
	if len(got) != SearchUsersLimit {
		t.Errorf("SearchUsers() returned %d users, want %d", len(got), SearchUsersLimit)
	}
}
