//go:build integration

package cache

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newTestRedisClient(t *testing.T) *redis.Client {
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

func TestLoginRateLimiter_AllowsUpToLimit(t *testing.T) {
	client := newTestRedisClient(t)
	limiter := NewLoginRateLimiter(client)
	ctx := context.Background()

	for i := 0; i < LoginRateLimitMax; i++ {
		allowed, err := limiter.Allow(ctx, "user@example.com")
		if err != nil {
			t.Fatalf("Allow() unexpected error on attempt %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("Allow() = false on attempt %d, want true (limit is %d)", i+1, LoginRateLimitMax)
		}
	}
}

func TestLoginRateLimiter_BlocksAfterLimit(t *testing.T) {
	client := newTestRedisClient(t)
	limiter := NewLoginRateLimiter(client)
	ctx := context.Background()

	for i := 0; i < LoginRateLimitMax; i++ {
		if _, err := limiter.Allow(ctx, "user@example.com"); err != nil {
			t.Fatalf("Allow() unexpected error: %v", err)
		}
	}

	allowed, err := limiter.Allow(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Allow() unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("Allow() = true after %d attempts, want false (limit exceeded)", LoginRateLimitMax)
	}
}

func TestLoginRateLimiter_KeysAreIndependent(t *testing.T) {
	client := newTestRedisClient(t)
	limiter := NewLoginRateLimiter(client)
	ctx := context.Background()

	for i := 0; i < LoginRateLimitMax; i++ {
		if _, err := limiter.Allow(ctx, "userA@example.com"); err != nil {
			t.Fatalf("Allow() unexpected error: %v", err)
		}
	}

	allowed, err := limiter.Allow(ctx, "userB@example.com")
	if err != nil {
		t.Fatalf("Allow() unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Allow() = false for a different key, want true (rate limits must not leak across keys)")
	}
}
