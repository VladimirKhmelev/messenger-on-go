//go:build integration

package cache

import (
	"context"
	"sync"
	"sync/atomic"
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

func TestTokenBlacklist_FirstClaimWins(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	ok, _, err := blacklist.Claim(ctx, "token", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first Claim() = %v, %v; want true", ok, err)
	}

	ok, usedAt, err := blacklist.Claim(ctx, "token", time.Minute)
	if err != nil || ok {
		t.Fatalf("second Claim() = %v, %v; want false", ok, err)
	}
	if time.Since(usedAt) > 5*time.Second {
		t.Errorf("usedAt = %v, want roughly now", usedAt)
	}
}

// Two tabs refreshing with the same cookie at the same moment: exactly one
// may get new tokens.
func TestTokenBlacklist_ConcurrentClaimsHaveOneWinner(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	const n = 20
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := blacklist.Claim(ctx, "shared-token", time.Minute)
			if err != nil {
				t.Errorf("Claim() error: %v", err)
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Errorf("%d of %d concurrent claims won, want exactly 1", wins.Load(), n)
	}
}

func TestTokenBlacklist_RevokedTokenCannotBeClaimed(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	if err := blacklist.Revoke(ctx, "logged-out", time.Minute); err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}
	if ok, _, err := blacklist.Claim(ctx, "logged-out", time.Minute); err != nil || ok {
		t.Errorf("Claim() after Revoke() = %v, %v; want false", ok, err)
	}
	if ok, _, err := blacklist.Claim(ctx, "other-token", time.Minute); err != nil || !ok {
		t.Errorf("Claim(unrelated token) = %v, %v; want true", ok, err)
	}
}

// Entries written before Claim existed hold "1" instead of a timestamp.
func TestTokenBlacklist_LegacyEntryCountsAsLongAgo(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	if err := client.Set(ctx, refreshBlacklistKeyPrefix+hashToken("old"), "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	ok, usedAt, err := blacklist.Claim(ctx, "old", time.Minute)
	if err != nil || ok || time.Since(usedAt) < time.Hour {
		t.Errorf("Claim(legacy) = %v, %v, %v; want false, long ago", ok, usedAt, err)
	}
}

func TestTokenBlacklist_ExpiresAfterTTL(t *testing.T) {
	client := newBlacklistTestRedisClient(t)
	blacklist := NewTokenBlacklist(client)
	ctx := context.Background()

	if ok, _, err := blacklist.Claim(ctx, "short-lived-token", time.Second); err != nil || !ok {
		t.Fatalf("Claim() = %v, %v; want true", ok, err)
	}

	time.Sleep(2 * time.Second)

	if ok, _, err := blacklist.Claim(ctx, "short-lived-token", time.Second); err != nil || !ok {
		t.Errorf("Claim() after TTL = %v, %v; want true", ok, err)
	}
}
