package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	emailVerifyRateLimitKeyPrefix = "auth:email-verify-attempts:"
	EmailVerifyRateLimitMax       = 10
	EmailVerifyRateLimitWindow    = 10 * time.Minute
)

type EmailVerifyRateLimiter struct {
	client *redis.Client
}

func NewEmailVerifyRateLimiter(client *redis.Client) *EmailVerifyRateLimiter {
	return &EmailVerifyRateLimiter{client: client}
}

func (l *EmailVerifyRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := emailVerifyRateLimitKeyPrefix + key

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, EmailVerifyRateLimitWindow).Err(); err != nil {
			return false, err
		}
	}

	return count <= EmailVerifyRateLimitMax, nil
}
