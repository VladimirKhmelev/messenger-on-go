package cache

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const passwordChangedKeyPrefix = "auth:password-changed-at:"

type PasswordChangeTracker struct {
	client *redis.Client
}

func NewPasswordChangeTracker(client *redis.Client) *PasswordChangeTracker {
	return &PasswordChangeTracker{client: client}
}

func (t *PasswordChangeTracker) MarkChanged(ctx context.Context, userID string, ttl time.Duration) error {
	return t.client.Set(ctx, passwordChangedKeyPrefix+userID, time.Now().Unix(), ttl).Err()
}

func (t *PasswordChangeTracker) ChangedAfter(ctx context.Context, userID string, issuedAt time.Time) (bool, error) {
	val, err := t.client.Get(ctx, passwordChangedKeyPrefix+userID).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	changedAtUnix, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false, err
	}

	return time.Unix(changedAtUnix, 0).After(issuedAt), nil
}
