//go:build integration

package cache

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newEmailVerificationTestRedisClient(t *testing.T) *redis.Client {
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

func TestEmailVerificationStore_GenerateAndVerify(t *testing.T) {
	client := newEmailVerificationTestRedisClient(t)
	store := NewEmailVerificationStore(client)
	ctx := context.Background()

	email := "user@example.com"
	code, err := store.GenerateAndStore(ctx, email)
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("GenerateAndStore() code = %q, want 6 digits", code)
	}

	ok, err := store.Verify(ctx, email, code)
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if !ok {
		t.Error("Verify() = false for the correct code, want true")
	}
}

func TestEmailVerificationStore_WrongCodeFails(t *testing.T) {
	client := newEmailVerificationTestRedisClient(t)
	store := NewEmailVerificationStore(client)
	ctx := context.Background()

	email := "user@example.com"
	if _, err := store.GenerateAndStore(ctx, email); err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	ok, err := store.Verify(ctx, email, "000000")
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if ok {
		t.Error("Verify() = true for a wrong code, want false")
	}
}

func TestEmailVerificationStore_CodeIsSingleUse(t *testing.T) {
	client := newEmailVerificationTestRedisClient(t)
	store := NewEmailVerificationStore(client)
	ctx := context.Background()

	email := "user@example.com"
	code, err := store.GenerateAndStore(ctx, email)
	if err != nil {
		t.Fatalf("GenerateAndStore() unexpected error: %v", err)
	}

	ok, err := store.Verify(ctx, email, code)
	if err != nil || !ok {
		t.Fatalf("first Verify() = %v, %v, want true, nil", ok, err)
	}

	ok, err = store.Verify(ctx, email, code)
	if err != nil {
		t.Fatalf("second Verify() unexpected error: %v", err)
	}
	if ok {
		t.Error("second Verify() = true for an already-used code, want false")
	}
}

func TestEmailVerificationStore_VerifyWithoutGenerateFails(t *testing.T) {
	client := newEmailVerificationTestRedisClient(t)
	store := NewEmailVerificationStore(client)
	ctx := context.Background()

	ok, err := store.Verify(ctx, "never-registered@example.com", "123456")
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if ok {
		t.Error("Verify() = true for an email with no stored code, want false")
	}
}
