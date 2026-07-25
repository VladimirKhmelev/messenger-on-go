//go:build integration

package cache

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newPasswordResetTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Errorf("failed to terminate redis container: %v", err)
		}
	})

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Fatalf("failed to parse redis connection string %q: %v", uri, err)
	}

	return redis.NewClient(opts)
}

func TestPasswordResetStore_GenerateAndConsume(t *testing.T) {
	client := newPasswordResetTestRedisClient(t)
	store := NewPasswordResetStore(client)
	ctx := context.Background()

	email := "user@example.com"
	token, err := store.GenerateAndStore(ctx, email)
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}
	if len(token) < 32 {
		t.Errorf("GenerateAndStore() token length = %d, want a long random token", len(token))
	}

	gotEmail, ok, err := store.Consume(ctx, token)
	if err != nil {
		t.Fatalf("Consume() unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("Consume() = false for a valid token, want true")
	}
	if gotEmail != email {
		t.Errorf("Consume() email = %q, want %q", gotEmail, email)
	}
}

func TestPasswordResetStore_TokenIsSingleUse(t *testing.T) {
	client := newPasswordResetTestRedisClient(t)
	store := NewPasswordResetStore(client)
	ctx := context.Background()

	token, err := store.GenerateAndStore(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	_, ok, err := store.Consume(ctx, token)
	if err != nil || !ok {
		t.Fatalf("first Consume() = %v, %v, want true, nil", ok, err)
	}

	_, ok, err = store.Consume(ctx, token)
	if err != nil {
		t.Fatalf("second Consume() unexpected error: %v", err)
	}
	if ok {
		t.Error("second Consume() = true for an already-used token, want false")
	}
}

func TestPasswordResetStore_UnknownTokenFails(t *testing.T) {
	client := newPasswordResetTestRedisClient(t)
	store := NewPasswordResetStore(client)
	ctx := context.Background()

	_, ok, err := store.Consume(ctx, "never-generated-token")
	if err != nil {
		t.Fatalf("Consume() unexpected error: %v", err)
	}
	if ok {
		t.Error("Consume() = true for a token that was never generated, want false")
	}
}
