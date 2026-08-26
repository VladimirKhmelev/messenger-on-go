package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

func TestAuthService_LoginWithGitHub_NewUser(t *testing.T) {
	repo := newFakeUserRepository()
	github := newFakeGitHubClient()
	github.profile = &oauth.GitHubProfile{ID: 42, Login: "octocat", Email: "octocat@example.com"}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, newFakeEventPublisher(), newFakePasswordChangeTracker())

	result, err := svc.LoginWithGitHub(context.Background(), "some-code", "pub-key", "wrapped-priv-key", "salt")
	if err != nil {
		t.Fatalf("LoginWithGitHub() unexpected error: %v", err)
	}
	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Error("LoginWithGitHub() returned empty tokens")
	}
	if !result.IsNewUser {
		t.Error("LoginWithGitHub() IsNewUser = false, want true for a brand-new GitHub account")
	}

	created := repo.users["octocat@example.com"]
	if created == nil {
		t.Fatal("LoginWithGitHub() did not create a user")
		return
	}
	if !created.EmailVerified {
		t.Error("LoginWithGitHub() created a user with EmailVerified = false, want true (GitHub already verified it)")
	}
	if created.Tag == "" {
		t.Error("LoginWithGitHub() created a user with an empty tag")
	}
	if created.PublicKey != "pub-key" || created.WrappedPrivateKey != "wrapped-priv-key" || created.KeyWrapSalt != "salt" {
		t.Errorf("LoginWithGitHub() created a user with keys = %+v, want the ones passed in", created)
	}
}

func TestAuthService_LoginWithGitHub_ExistingUser(t *testing.T) {
	repo := newFakeUserRepository()
	existing := newTestUser("octocat@example.com", "abcd1234")
	repo.users[existing.Email] = existing

	github := newFakeGitHubClient()
	github.profile = &oauth.GitHubProfile{ID: 42, Login: "octocat", Email: "octocat@example.com"}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, newFakeEventPublisher(), newFakePasswordChangeTracker())

	result, err := svc.LoginWithGitHub(context.Background(), "some-code", "", "", "")
	if err != nil {
		t.Fatalf("LoginWithGitHub() unexpected error: %v", err)
	}
	if result.IsNewUser {
		t.Error("LoginWithGitHub() IsNewUser = true, want false for an existing account")
	}

	issuer := jwtutil.NewIssuer("test-secret")
	claims, err := issuer.Parse(result.Tokens.AccessToken, jwtutil.TokenTypeAccess)
	if err != nil {
		t.Fatalf("access token failed to parse: %v", err)
	}
	if claims.UserID != existing.ID {
		t.Errorf("LoginWithGitHub() issued tokens for UserID = %q, want %q (should link to the existing account)", claims.UserID, existing.ID)
	}
}

func TestAuthService_LoginWithGitHub_PropagatesGitHubError(t *testing.T) {
	repo := newFakeUserRepository()
	github := newFakeGitHubClient()
	github.err = errors.New("github oauth failed")
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, newFakeEventPublisher(), newFakePasswordChangeTracker())

	_, err := svc.LoginWithGitHub(context.Background(), "bad-code", "pub-key", "wrapped-priv-key", "salt")
	if err == nil {
		t.Fatal("LoginWithGitHub() expected an error, got nil")
	}
}

func TestAuthService_LoginWithGitHub_NoVerifiedEmail(t *testing.T) {
	repo := newFakeUserRepository()
	github := newFakeGitHubClient()
	github.err = domain.ErrOAuthNoVerifiedEmail
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, newFakeEventPublisher(), newFakePasswordChangeTracker())

	_, err := svc.LoginWithGitHub(context.Background(), "some-code", "pub-key", "wrapped-priv-key", "salt")
	if !errors.Is(err, domain.ErrOAuthNoVerifiedEmail) {
		t.Errorf("LoginWithGitHub() error = %v, want %v", err, domain.ErrOAuthNoVerifiedEmail)
	}
}
