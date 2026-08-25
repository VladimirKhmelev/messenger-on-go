package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

var errPublishFailed = errors.New("publish failed")

func TestAuthService_Register_PublishesUserRegistered(t *testing.T) {
	repo := newFakeUserRepository()
	publisher := newFakeEventPublisher()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), publisher, newFakePasswordChangeTracker())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if len(publisher.registeredEvents) != 1 {
		t.Fatalf("Register() published %d user.registered events, want 1", len(publisher.registeredEvents))
	}
	event := publisher.registeredEvents[0]
	if event.UserID != user.ID || event.Email != user.Email || event.Tag != user.Tag {
		t.Errorf("Register() published event = %+v, want to match user %+v", event, user)
	}
}

func TestAuthService_Register_EventPublishFailureDoesNotFailRegistration(t *testing.T) {
	repo := newFakeUserRepository()
	publisher := &failingEventPublisher{}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), publisher, newFakePasswordChangeTracker())

	_, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v, want nil (event publish failures must not fail registration)", err)
	}
}

func TestAuthService_ResetPassword_PublishesUserPasswordReset(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser("user@example.com", "oldpass1")
	repo.users[user.Email] = user

	resets := newFakePasswordResetStore()
	publisher := newFakeEventPublisher()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), resets, newFakeGitHubClient(), publisher, newFakePasswordChangeTracker())

	token, err := resets.GenerateAndStore(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	if err := svc.ResetPassword(context.Background(), token, "newpass1", "test-public-key", "test-wrapped-key", "test-salt"); err != nil {
		t.Fatalf("ResetPassword() unexpected error: %v", err)
	}

	if len(publisher.passwordResetEvents) != 1 {
		t.Fatalf("ResetPassword() published %d user.password_reset events, want 1", len(publisher.passwordResetEvents))
	}
	if publisher.passwordResetEvents[0].UserID != user.ID {
		t.Errorf("published event UserID = %q, want %q", publisher.passwordResetEvents[0].UserID, user.ID)
	}
}

func TestAuthService_LoginWithGitHub_PublishesUserOAuthLinkedForNewUser(t *testing.T) {
	repo := newFakeUserRepository()
	github := newFakeGitHubClient()
	github.profile = &oauth.GitHubProfile{ID: 42, Login: "octocat", Email: "octocat@example.com"}
	publisher := newFakeEventPublisher()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, publisher, newFakePasswordChangeTracker())

	_, err := svc.LoginWithGitHub(context.Background(), "some-code", "pub-key", "wrapped-priv-key", "salt")
	if err != nil {
		t.Fatalf("LoginWithGitHub() unexpected error: %v", err)
	}

	if len(publisher.oauthLinkedEvents) != 1 {
		t.Fatalf("LoginWithGitHub() published %d user.oauth_linked events, want 1", len(publisher.oauthLinkedEvents))
	}
	if publisher.oauthLinkedEvents[0].Provider != "github" {
		t.Errorf("published event Provider = %q, want %q", publisher.oauthLinkedEvents[0].Provider, "github")
	}
}

func TestAuthService_LoginWithGitHub_NoEventForExistingUser(t *testing.T) {
	repo := newFakeUserRepository()
	existing := newTestUser("octocat@example.com", "abcd1234")
	repo.users[existing.Email] = existing

	github := newFakeGitHubClient()
	github.profile = &oauth.GitHubProfile{ID: 42, Login: "octocat", Email: "octocat@example.com"}
	publisher := newFakeEventPublisher()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, publisher, newFakePasswordChangeTracker())

	_, err := svc.LoginWithGitHub(context.Background(), "some-code", "", "", "")
	if err != nil {
		t.Fatalf("LoginWithGitHub() unexpected error: %v", err)
	}

	if len(publisher.oauthLinkedEvents) != 0 {
		t.Errorf("LoginWithGitHub() published %d user.oauth_linked events for an existing user, want 0", len(publisher.oauthLinkedEvents))
	}
}

type failingEventPublisher struct{}

func (failingEventPublisher) PublishUserRegistered(context.Context, events.UserRegistered) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishUserPasswordReset(context.Context, events.UserPasswordReset) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishUserOAuthLinked(context.Context, events.UserOAuthLinked) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishUserProfileUpdated(context.Context, events.UserProfileUpdated) error {
	return errPublishFailed
}
