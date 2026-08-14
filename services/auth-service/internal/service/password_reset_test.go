package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func TestAuthService_RequestPasswordReset_ExistingUser(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "abcd1234")
	repo.users[user.Email] = user

	mailer := newFakeMailer()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), mailer, newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	if err := svc.RequestPasswordReset(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() unexpected error: %v", err)
	}

	if mailer.sentResetTo != "user@example.com" {
		t.Errorf("RequestPasswordReset() sent reset email to %q, want %q", mailer.sentResetTo, "user@example.com")
	}
	if mailer.sentResetToken == "" {
		t.Error("RequestPasswordReset() did not send a reset token")
	}
}

func TestAuthService_RequestPasswordReset_UnknownEmail_StillSucceeds(t *testing.T) {
	repo := newFakeUserRepository()
	mailer := newFakeMailer()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), mailer, newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	err := svc.RequestPasswordReset(context.Background(), "missing@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset() unexpected error: %v, want nil (must not leak whether the email exists)", err)
	}

	if mailer.sentResetTo != "" {
		t.Error("RequestPasswordReset() sent an email for a nonexistent account")
	}
}

func TestAuthService_ResetPassword_Success(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "oldpass1")
	repo.users[user.Email] = user

	resets := newFakePasswordResetStore()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), resets, newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	if err := svc.RequestPasswordReset(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() unexpected error: %v", err)
	}

	var token string
	for tok, email := range resets.tokens {
		if email == "user@example.com" {
			token = tok
		}
	}
	if token == "" {
		t.Fatal("no reset token was stored")
	}

	if err := svc.ResetPassword(context.Background(), token, "newpass1"); err != nil {
		t.Fatalf("ResetPassword() unexpected error: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newpass1")); err != nil {
		t.Errorf("password was not updated: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("oldpass1")); err == nil {
		t.Error("old password still matches after reset")
	}
}

func TestAuthService_ResetPassword_TokenIsSingleUse(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "oldpass1")
	repo.users[user.Email] = user

	resets := newFakePasswordResetStore()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), resets, newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	token, err := resets.GenerateAndStore(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	if err := svc.ResetPassword(context.Background(), token, "newpass1"); err != nil {
		t.Fatalf("first ResetPassword() unexpected error: %v", err)
	}

	err = svc.ResetPassword(context.Background(), token, "anotherpass1")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("second ResetPassword() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestAuthService_ResetPassword_InvalidToken(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	err := svc.ResetPassword(context.Background(), "not-a-real-token", "newpass1")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("ResetPassword() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestAuthService_ResetPassword_WeakPassword(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "oldpass1")
	repo.users[user.Email] = user

	resets := newFakePasswordResetStore()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), resets, newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	token, err := resets.GenerateAndStore(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	err = svc.ResetPassword(context.Background(), token, "weak")
	if !errors.Is(err, domain.ErrWeakPassword) {
		t.Errorf("ResetPassword() error = %v, want %v", err, domain.ErrWeakPassword)
	}
}
