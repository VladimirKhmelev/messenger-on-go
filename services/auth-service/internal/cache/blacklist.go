package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	return b.client.Set(ctx, refreshBlacklistKeyPrefix+hashToken(token), "1", ttl).Err()
}

func (b *TokenBlacklist) IsRevoked(ctx context.Context, token string) (bool, error) {
	n, err := b.client.Exists(ctx, refreshBlacklistKeyPrefix+hashToken(token)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
