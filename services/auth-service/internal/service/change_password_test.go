package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func TestAuthService_ChangePassword_Success(t *testing.T) {
	repo := newFakeUserRepository()
	mailer := newFakeMailer()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), mailer, newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	repo.users[user.Email].EmailVerified = true

	if err := svc.ChangePassword(context.Background(), user.ID, "abcd1234", "newpass9"); err != nil {
		t.Fatalf("ChangePassword() unexpected error: %v", err)
	}

	if _, err := svc.Login(context.Background(), "user@example.com", "newpass9"); err != nil {
		t.Errorf("Login() with new password unexpected error: %v", err)
	}

	if mailer.sentPasswordChange != "user@example.com" {
		t.Errorf("SendPasswordChanged() called with %q, want %q", mailer.sentPasswordChange, "user@example.com")
	}
}

func TestAuthService_ChangePassword_WrongOldPassword(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err = svc.ChangePassword(context.Background(), user.ID, "wrongpass1", "newpass9")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthService_ChangePassword_WeakNewPassword(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err = svc.ChangePassword(context.Background(), user.ID, "abcd1234", "weak")
	if !errors.Is(err, domain.ErrWeakPassword) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrWeakPassword)
	}
}

func TestAuthService_ChangePassword_SameAsOld(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err = svc.ChangePassword(context.Background(), user.ID, "abcd1234", "abcd1234")
	if !errors.Is(err, domain.ErrSamePassword) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrSamePassword)
	}
}

func TestAuthService_ChangePassword_RateLimited(t *testing.T) {
	repo := newFakeUserRepository()
	limiter := &fakeRateLimiter{allow: true}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), limiter, newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	limiter.allow = false

	err = svc.ChangePassword(context.Background(), user.ID, "wrongpass1", "newpass9")
	if !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrTooManyAttempts)
	}
}
