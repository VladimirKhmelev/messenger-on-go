package service

import (
	"context"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

func TestAuthService_DeleteAccount_Success(t *testing.T) {
	repo := newFakeUserRepository()
	refreshRevoked := newFakeRefreshRevokedTracker()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker(), refreshRevoked)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	repo.users[user.Email].EmailVerified = true

	if err := svc.DeleteAccount(context.Background(), user.ID, "abcd1234"); err != nil {
		t.Fatalf("DeleteAccount() unexpected error: %v", err)
	}

	got, err := svc.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() unexpected error: %v", err)
	}
	if !got.Deleted {
		t.Error("DeleteAccount() left Deleted = false, want true")
	}
	if got.Email == "user@example.com" {
		t.Error("DeleteAccount() did not anonymize the email")
	}
	if got.Tag == "balbes" {
		t.Error("DeleteAccount() did not anonymize the tag")
	}
	if got.DisplayName == "Name" {
		t.Error("DeleteAccount() did not anonymize the display name")
	}
	if got.PublicKey != "" || got.WrappedPrivateKey != "" || got.KeyWrapSalt != "" {
		t.Error("DeleteAccount() did not wipe E2E key material")
	}

	emailTaken, err := repo.ExistsByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("ExistsByEmail() unexpected error: %v", err)
	}
	if emailTaken {
		t.Error("DeleteAccount() left the original email marked as taken")
	}
	tagTaken, err := repo.ExistsByTag(context.Background(), "balbes")
	if err != nil {
		t.Fatalf("ExistsByTag() unexpected error: %v", err)
	}
	if tagTaken {
		t.Error("DeleteAccount() left the original tag marked as taken")
	}

	if _, ok := refreshRevoked.revokedAt[user.ID]; !ok {
		t.Error("DeleteAccount() did not revoke the user's refresh tokens")
	}
}

func TestAuthService_DeleteAccount_OAuthAccountNoPasswordRequired(t *testing.T) {
	repo := newFakeUserRepository()
	github := newFakeGitHubClient()
	github.profile = &oauth.GitHubProfile{ID: 42, Login: "octocat", Email: "octocat@example.com"}
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), github, newFakeEventPublisher(), newFakePasswordChangeTracker(), newFakeRefreshRevokedTracker())

	result, err := svc.LoginWithGitHub(context.Background(), "some-code", "pub-key", "wrapped-priv-key", "salt")
	if err != nil {
		t.Fatalf("LoginWithGitHub() unexpected error: %v", err)
	}
	created := repo.users["octocat@example.com"]

	if err := svc.DeleteAccount(context.Background(), created.ID, ""); err != nil {
		t.Fatalf("DeleteAccount() unexpected error for OAuth account: %v", err)
	}

	got, err := svc.GetUserByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetUserByID() unexpected error: %v", err)
	}
	if !got.Deleted {
		t.Error("DeleteAccount() did not delete the OAuth account")
	}

	_ = result
}
