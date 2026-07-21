package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func newTestUser(email, password string) *domain.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return &domain.User{
		ID:           "user-1",
		Email:        email,
		Tag:          "john_doe",
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "abcd1234")
	repo.users[user.Email] = user
	svc := newTestAuthService(repo)

	tokens, err := svc.Login(context.Background(), "user@example.com", "abcd1234")
	if err != nil {
		t.Fatalf("Login() unexpected error: %v", err)
	}

	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Error("Login() returned empty tokens")
	}

	issuer := jwtutil.NewIssuer("test-secret")

	accessClaims, err := issuer.Parse(tokens.AccessToken, jwtutil.TokenTypeAccess)
	if err != nil {
		t.Fatalf("access token failed to parse: %v", err)
	}
	if accessClaims.UserID != user.ID {
		t.Errorf("access token UserID = %q, want %q", accessClaims.UserID, user.ID)
	}

	refreshClaims, err := issuer.Parse(tokens.RefreshToken, jwtutil.TokenTypeRefresh)
	if err != nil {
		t.Fatalf("refresh token failed to parse: %v", err)
	}
	if refreshClaims.UserID != user.ID {
		t.Errorf("refresh token UserID = %q, want %q", refreshClaims.UserID, user.ID)
	}
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.Login(context.Background(), "missing@example.com", "abcd1234")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Login() error = %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "abcd1234")
	repo.users[user.Email] = user
	svc := newTestAuthService(repo)

	_, err := svc.Login(context.Background(), "user@example.com", "wrongpass1")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Login() error = %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthService_Login_RateLimited(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "abcd1234")
	repo.users[user.Email] = user

	limiter := newFakeRateLimiter()
	limiter.allow = false
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), limiter)

	_, err := svc.Login(context.Background(), "user@example.com", "abcd1234")
	if !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Errorf("Login() error = %v, want %v", err, domain.ErrTooManyAttempts)
	}
}

func TestAuthService_RefreshToken_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	issuer := jwtutil.NewIssuer("test-secret")
	oldRefresh, err := issuer.IssueRefreshToken("user-1")
	if err != nil {
		t.Fatalf("IssueRefreshToken() unexpected error: %v", err)
	}

	tokens, err := svc.RefreshToken(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("RefreshToken() unexpected error: %v", err)
	}

	accessClaims, err := issuer.Parse(tokens.AccessToken, jwtutil.TokenTypeAccess)
	if err != nil {
		t.Fatalf("new access token failed to parse: %v", err)
	}
	if accessClaims.UserID != "user-1" {
		t.Errorf("new access token UserID = %q, want %q", accessClaims.UserID, "user-1")
	}

	refreshClaims, err := issuer.Parse(tokens.RefreshToken, jwtutil.TokenTypeRefresh)
	if err != nil {
		t.Fatalf("new refresh token failed to parse: %v", err)
	}
	if refreshClaims.UserID != "user-1" {
		t.Errorf("new refresh token UserID = %q, want %q", refreshClaims.UserID, "user-1")
	}
}

func TestAuthService_RefreshToken_RejectsAccessToken(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	issuer := jwtutil.NewIssuer("test-secret")
	accessToken, err := issuer.IssueAccessToken("user-1")
	if err != nil {
		t.Fatalf("IssueAccessToken() unexpected error: %v", err)
	}

	_, err = svc.RefreshToken(context.Background(), accessToken)
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("RefreshToken() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestAuthService_RefreshToken_MalformedToken(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.RefreshToken(context.Background(), "not-a-valid-token")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("RefreshToken() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}
