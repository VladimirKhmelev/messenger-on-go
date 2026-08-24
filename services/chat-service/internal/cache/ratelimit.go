package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sendMessageRateLimitKeyPrefix = "chat:send-attempts:"
	SendMessageRateLimitMax       = 30
	SendMessageRateLimitWindow    = time.Minute
)

type SendMessageRateLimiter struct {
	client *redis.Client
}

func NewSendMessageRateLimiter(client *redis.Client) *SendMessageRateLimiter {
	return &SendMessageRateLimiter{client: client}
}

func (l *SendMessageRateLimiter) Allow(ctx context.Context, userID string) (bool, error) {
	redisKey := sendMessageRateLimitKeyPrefix + userID

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, SendMessageRateLimitWindow).Err(); err != nil {
			return false, err
		}
	}

	return count <= SendMessageRateLimitMax, nil
}
