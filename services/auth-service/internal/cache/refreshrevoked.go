package cache

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const refreshRevokedAllKeyPrefix = "auth:refresh-revoked-all-at:"

type RefreshRevokedTracker struct {
	client *redis.Client
}

func NewRefreshRevokedTracker(client *redis.Client) *RefreshRevokedTracker {
	return &RefreshRevokedTracker{client: client}
}

func (t *RefreshRevokedTracker) MarkAllRevoked(ctx context.Context, userID string, ttl time.Duration) error {
	return t.client.Set(ctx, refreshRevokedAllKeyPrefix+userID, time.Now().Unix(), ttl).Err()
}

func (t *RefreshRevokedTracker) RevokedAfter(ctx context.Context, userID string, issuedAt time.Time) (bool, error) {
	val, err := t.client.Get(ctx, refreshRevokedAllKeyPrefix+userID).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	revokedAtUnix, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false, err
	}

	return time.Unix(revokedAtUnix, 0).After(issuedAt), nil
}
