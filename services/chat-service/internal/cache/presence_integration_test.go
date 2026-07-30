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

func TestPresenceStore_SetOnline_IsOnline(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewPresenceStore(client)
	ctx := context.Background()

	online, err := store.IsOnline(ctx, "user-a")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if online {
		t.Fatal("IsOnline() = true before SetOnline, want false")
	}

	if err := store.SetOnline(ctx, "user-a"); err != nil {
		t.Fatalf("SetOnline() unexpected error: %v", err)
	}

	online, err = store.IsOnline(ctx, "user-a")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if !online {
		t.Error("IsOnline() = false after SetOnline, want true")
	}
}

func TestPresenceStore_SetOffline(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewPresenceStore(client)
	ctx := context.Background()

	if err := store.SetOnline(ctx, "user-a"); err != nil {
		t.Fatalf("SetOnline() unexpected error: %v", err)
	}

	if err := store.SetOffline(ctx, "user-a"); err != nil {
		t.Fatalf("SetOffline() unexpected error: %v", err)
	}

	online, err := store.IsOnline(ctx, "user-a")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if online {
		t.Error("IsOnline() = true after SetOffline, want false")
	}
}

func TestPresenceStore_KeysAreIndependent(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewPresenceStore(client)
	ctx := context.Background()

	if err := store.SetOnline(ctx, "user-a"); err != nil {
		t.Fatalf("SetOnline() unexpected error: %v", err)
	}

	online, err := store.IsOnline(ctx, "user-b")
	if err != nil {
		t.Fatalf("IsOnline() unexpected error: %v", err)
	}
	if online {
		t.Error("IsOnline() = true for a different user, want false (presence must not leak across keys)")
	}
}
