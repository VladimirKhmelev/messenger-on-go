//go:build integration

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newBlacklistTestRedisClient(t *testing.T) *redis.Client {
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

func TestTokenBlacklist_NotRevokedByDefault(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	revoked, err := blacklist.IsRevoked(ctx, "some-refresh-token")
	if err != nil {
		t.Fatalf("IsRevoked() unexpected error: %v", err)
	}
	if revoked {
		t.Error("IsRevoked() = true for a token that was never revoked, want false")
	}
}

func TestTokenBlacklist_RevokeMarksTokenAsRevoked(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	token := "some-refresh-token"
	if err := blacklist.Revoke(ctx, token, time.Minute); err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}

	revoked, err := blacklist.IsRevoked(ctx, token)
	if err != nil {
		t.Fatalf("IsRevoked() unexpected error: %v", err)
	}
	if !revoked {
		t.Error("IsRevoked() = false after Revoke(), want true")
	}
}

func TestTokenBlacklist_DifferentTokensAreIndependent(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	if err := blacklist.Revoke(ctx, "token-a", time.Minute); err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}

	revoked, err := blacklist.IsRevoked(ctx, "token-b")
	if err != nil {
		t.Fatalf("IsRevoked() unexpected error: %v", err)
	}
	if revoked {
		t.Error("IsRevoked() = true for an unrelated token, want false (revocations must not leak across tokens)")
	}
}

func TestTokenBlacklist_ExpiresAfterTTL(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	token := "short-lived-token"
	if err := blacklist.Revoke(ctx, token, time.Second); err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}

	time.Sleep(2 * time.Second)

	revoked, err := blacklist.IsRevoked(ctx, token)
	if err != nil {
		t.Fatalf("IsRevoked() unexpected error: %v", err)
	}
	if revoked {
		t.Error("IsRevoked() = true after TTL expired, want false")
	}
}
