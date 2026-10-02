package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const refreshBlacklistKeyPrefix = "auth:refresh-blacklist:"

type TokenBlacklist struct {
	client *redis.Client
}

func NewTokenBlacklist(client *redis.Client) *TokenBlacklist {
	return &TokenBlacklist{client: client}
}

func (b *TokenBlacklist) Revoke(ctx context.Context, token string, ttl time.Duration) error {
	return b.client.Set(ctx, refreshBlacklistKeyPrefix+hashToken(token), time.Now().Unix(), ttl).Err()
}

func (b *TokenBlacklist) Claim(ctx context.Context, token string, ttl time.Duration) (ok bool, usedAt time.Time, err error) {
	prev, err := b.client.SetArgs(ctx, refreshBlacklistKeyPrefix+hashToken(token), time.Now().Unix(), redis.SetArgs{
		Mode: "NX",
		Get:  true,
		TTL:  ttl,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return true, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}

	unix, err := strconv.ParseInt(prev, 10, 64)
	if err != nil {
		return false, time.Unix(0, 0), nil
	}
	return false, time.Unix(unix, 0), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
