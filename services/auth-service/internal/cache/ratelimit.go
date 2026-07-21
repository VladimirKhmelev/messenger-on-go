package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	loginRateLimitKeyPrefix = "auth:login-attempts:"
	LoginRateLimitMax       = 10
	LoginRateLimitWindow    = time.Minute
)

type LoginRateLimiter struct {
	client *redis.Client
}

func NewLoginRateLimiter(client *redis.Client) *LoginRateLimiter {
	return &LoginRateLimiter{client: client}
}

func (l *LoginRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := loginRateLimitKeyPrefix + key

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, LoginRateLimitWindow).Err(); err != nil {
			return false, err
		}
	}

	return count <= LoginRateLimitMax, nil
}
