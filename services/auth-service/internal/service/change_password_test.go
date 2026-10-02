package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func TestAuthService_ChangePassword_Success(t *testing.T) {
	repo := newFakeUserRepository()
	mailer := newFakeMailer()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), mailer, newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker(), newFakeRefreshRevokedTracker(), newFakeRateLimiter())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	repo.users[user.Email].EmailVerified = true

	if _, err := svc.ChangePassword(context.Background(), user.ID, "abcd1234", "newpass9", "new-wrapped-key", "new-salt"); err != nil {
		t.Fatalf("ChangePassword() unexpected error: %v", err)
	}

	if _, err := svc.Login(context.Background(), "user@example.com", "newpass9"); err != nil {
		t.Errorf("Login() with new password unexpected error: %v", err)
	}

	if got := repo.users[user.Email]; got.WrappedPrivateKey != "new-wrapped-key" || got.KeyWrapSalt != "new-salt" {
		t.Errorf("wrapped key = %q/%q, want new-wrapped-key/new-salt", got.WrappedPrivateKey, got.KeyWrapSalt)
	}
	if got := repo.users[user.Email].PublicKey; got != "test-public-key" {
		t.Errorf("public key = %q, want unchanged test-public-key: a password change keeps the key pair", got)
	}

	if mailer.sentPasswordChange != "user@example.com" {
		t.Errorf("SendPasswordChanged() called with %q, want %q", mailer.sentPasswordChange, "user@example.com")
	}
}

func TestAuthService_ChangePassword_WrongOldPassword(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	_, err = svc.ChangePassword(context.Background(), user.ID, "wrongpass1", "newpass9", "new-wrapped-key", "new-salt")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthService_ChangePassword_WeakNewPassword(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	_, err = svc.ChangePassword(context.Background(), user.ID, "abcd1234", "weak", "new-wrapped-key", "new-salt")
	if !errors.Is(err, domain.ErrWeakPassword) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrWeakPassword)
	}
}

func TestAuthService_ChangePassword_SameAsOld(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	_, err = svc.ChangePassword(context.Background(), user.ID, "abcd1234", "abcd1234", "new-wrapped-key", "new-salt")
	if !errors.Is(err, domain.ErrSamePassword) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrSamePassword)
	}
}

func TestAuthService_ChangePassword_RateLimited(t *testing.T) {
	repo := newFakeUserRepository()
	limiter := &fakeRateLimiter{allow: true}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), limiter, newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker(), newFakeRefreshRevokedTracker(), newFakeRateLimiter())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	limiter.allow = false

	_, err = svc.ChangePassword(context.Background(), user.ID, "wrongpass1", "newpass9", "new-wrapped-key", "new-salt")
	if !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Errorf("ChangePassword() error = %v, want %v", err, domain.ErrTooManyAttempts)
	}
}

func TestAuthService_ChangePassword_InvalidatesTokensIssuedBefore(t *testing.T) {
	repo := newFakeUserRepository()
	tracker := newFakePasswordChangeTracker()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), tracker, newFakeRefreshRevokedTracker(), newFakeRateLimiter())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	// JWT iat has whole-second precision, like the tracker
	issuedBeforeChange := time.Now().Add(-time.Second)

	if _, err := svc.ChangePassword(context.Background(), user.ID, "abcd1234", "newpass9", "new-wrapped-key", "new-salt"); err != nil {
		t.Fatalf("ChangePassword() unexpected error: %v", err)
	}

	stale, err := svc.IsAccessTokenStale(context.Background(), user.ID, issuedBeforeChange)
	if err != nil {
		t.Fatalf("IsAccessTokenStale() unexpected error: %v", err)
	}
	if !stale {
		t.Error("IsAccessTokenStale() = false, want true for a token issued before the password change")
	}

	stillValid, err := svc.IsAccessTokenStale(context.Background(), user.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("IsAccessTokenStale() unexpected error: %v", err)
	}
	if stillValid {
		t.Error("IsAccessTokenStale() = true, want false for a token issued after the password change")
	}
}
