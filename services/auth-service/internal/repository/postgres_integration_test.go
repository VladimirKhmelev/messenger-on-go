//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func newTestRepository(t *testing.T) *PostgresUserRepository {
	t.Helper()

	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("auth_test"),
		postgres.WithUsername("auth_test"),
		postgres.WithPassword("auth_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Errorf("failed to terminate postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	repo, err := NewPostgresUserRepository(dsn)
	if err != nil {
		t.Fatalf("failed to connect repository: %v", err)
	}

	if err := repo.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return repo
}

func newIntegrationTestUser(email, tag string) *domain.User {
	return &domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		Tag:          tag,
		PasswordHash: "hashed-password",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

func TestPostgresUserRepository_CreateAndGetByEmail(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user := newIntegrationTestUser("user@example.com", "john_from_manhattan")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, err := repo.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetByEmail() unexpected error: %v", err)
	}
	if got.ID != user.ID || got.Tag != user.Tag {
		t.Errorf("GetByEmail() = %+v, want ID=%q Tag=%q", got, user.ID, user.Tag)
	}
}

func TestPostgresUserRepository_GetByEmail_NotFound(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	_, err := repo.GetByEmail(ctx, "missing@example.com")
	if err != domain.ErrUserNotFound {
		t.Errorf("GetByEmail() error = %v, want %v", err, domain.ErrUserNotFound)
	}
}

func TestPostgresUserRepository_GetByTag(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user := newIntegrationTestUser("tagged@example.com", "unique_tag")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, err := repo.GetByTag(ctx, user.Tag)
	if err != nil {
		t.Fatalf("GetByTag() unexpected error: %v", err)
	}
	if got.Email != user.Email {
		t.Errorf("GetByTag() Email = %q, want %q", got.Email, user.Email)
	}
}

func TestPostgresUserRepository_ExistsByEmailAndTag(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user := newIntegrationTestUser("exists@example.com", "exists_tag")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	emailExists, err := repo.ExistsByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("ExistsByEmail() unexpected error: %v", err)
	}
	if !emailExists {
		t.Error("ExistsByEmail() = false, want true")
	}

	tagExists, err := repo.ExistsByTag(ctx, user.Tag)
	if err != nil {
		t.Fatalf("ExistsByTag() unexpected error: %v", err)
	}
	if !tagExists {
		t.Error("ExistsByTag() = false, want true")
	}

	missingExists, err := repo.ExistsByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("ExistsByEmail() unexpected error: %v", err)
	}
	if missingExists {
		t.Error("ExistsByEmail() = true for nonexistent email, want false")
	}
}

func TestPostgresUserRepository_SearchByTagPrefix(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	for _, tag := range []string{"search_alice", "search_alan", "search_eva"} {
		user := newIntegrationTestUser(tag+"@example.com", tag)
		if err := repo.Create(ctx, user); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	}

	got, err := repo.SearchByTagPrefix(ctx, "search_al", 10)
	if err != nil {
		t.Fatalf("SearchByTagPrefix() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SearchByTagPrefix() returned %d users, want 2", len(got))
	}
	for _, u := range got {
		if u.Tag != "search_alice" && u.Tag != "search_alan" {
			t.Errorf("SearchByTagPrefix() returned unexpected tag %q", u.Tag)
		}
	}
}

func TestPostgresUserRepository_PasswordAndKeysChangeTogether(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user := newIntegrationTestUser("keys@example.com", "keys_owner")
	user.PublicKey, user.WrappedPrivateKey, user.KeyWrapSalt = "pub-1", "wrapped-1", "salt-1"
	other := newIntegrationTestUser("other@example.com", "other_user")
	other.PublicKey, other.WrappedPrivateKey, other.KeyWrapSalt = "other-pub", "other-wrapped", "other-salt"
	for _, u := range []*domain.User{user, other} {
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	}

	// password change re-wraps the same key pair
	if err := repo.UpdatePasswordAndWrappedKey(ctx, user.ID, "hash-2", "wrapped-2", "salt-2"); err != nil {
		t.Fatalf("UpdatePasswordAndWrappedKey() unexpected error: %v", err)
	}
	got, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() unexpected error: %v", err)
	}
	if got.PasswordHash != "hash-2" || got.WrappedPrivateKey != "wrapped-2" || got.KeyWrapSalt != "salt-2" || got.PublicKey != "pub-1" {
		t.Errorf("after password change: hash=%q wrapped=%q salt=%q pub=%q; want hash-2/wrapped-2/salt-2 and pub-1 kept",
			got.PasswordHash, got.WrappedPrivateKey, got.KeyWrapSalt, got.PublicKey)
	}

	// password reset replaces the key pair
	if err := repo.UpdatePasswordAndKeyPair(ctx, user.ID, "hash-3", "pub-3", "wrapped-3", "salt-3"); err != nil {
		t.Fatalf("UpdatePasswordAndKeyPair() unexpected error: %v", err)
	}
	got, _ = repo.GetByID(ctx, user.ID)
	if got.PasswordHash != "hash-3" || got.PublicKey != "pub-3" || got.WrappedPrivateKey != "wrapped-3" || got.KeyWrapSalt != "salt-3" {
		t.Errorf("after reset: hash=%q pub=%q wrapped=%q salt=%q; want all *-3",
			got.PasswordHash, got.PublicKey, got.WrappedPrivateKey, got.KeyWrapSalt)
	}

	untouched, _ := repo.GetByID(ctx, other.ID)
	if untouched.PasswordHash != "hashed-password" || untouched.PublicKey != "other-pub" || untouched.WrappedPrivateKey != "other-wrapped" {
		t.Errorf("other user changed: %+v", untouched)
	}
}

// Rows written before emails were normalized may be mixed-case.
func TestPostgresUserRepository_EmailLookupIgnoresCase(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	legacy := newIntegrationTestUser("Ivan@Example.COM", "ivan_legacy")
	if err := repo.Create(ctx, legacy); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, err := repo.GetByEmail(ctx, "ivan@example.com")
	if err != nil || got.ID != legacy.ID {
		t.Errorf("GetByEmail(lowercase) = %v, %v; want the legacy row", got, err)
	}
	if exists, err := repo.ExistsByEmail(ctx, "IVAN@example.com"); err != nil || !exists {
		t.Errorf("ExistsByEmail(other case) = %v, %v; want true", exists, err)
	}
}

// Two requests racing past the service's existence check: the loser must get
// the same domain error the check would have returned, not an internal one.
func TestPostgresUserRepository_UniqueViolationsAreDomainErrors(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	first := newIntegrationTestUser("first@example.com", "taken_tag")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if err := repo.Create(ctx, newIntegrationTestUser("first@example.com", "other_tag")); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("Create(duplicate email) err = %v, want %v", err, domain.ErrEmailTaken)
	}
	if err := repo.Create(ctx, newIntegrationTestUser("second@example.com", "taken_tag")); !errors.Is(err, domain.ErrTagTaken) {
		t.Errorf("Create(duplicate tag) err = %v, want %v", err, domain.ErrTagTaken)
	}

	second := newIntegrationTestUser("third@example.com", "free_tag")
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if err := repo.UpdateTag(ctx, second.ID, "taken_tag"); !errors.Is(err, domain.ErrTagTaken) {
		t.Errorf("UpdateTag(taken) err = %v, want %v", err, domain.ErrTagTaken)
	}
}
