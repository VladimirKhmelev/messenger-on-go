//go:build integration

package repository

import (
	"context"
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
