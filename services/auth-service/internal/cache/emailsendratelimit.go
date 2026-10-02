package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Verification codes and reset links are mailed to whatever address the
// caller types in, so without a limit anyone could flood a stranger's inbox
// and burn the SMTP sender's reputation.
const (
	emailSendRateLimitKeyPrefix = "auth:email-sends:"
	EmailSendRateLimitMax       = 3
	EmailSendRateLimitWindow    = 15 * time.Minute
)

type EmailSendRateLimiter struct {
	client *redis.Client
}

func NewEmailSendRateLimiter(client *redis.Client) *EmailSendRateLimiter {
	return &EmailSendRateLimiter{client: client}
}

func (l *EmailSendRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := emailSendRateLimitKeyPrefix + key

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, EmailSendRateLimitWindow).Err(); err != nil {
			return false, err
		}
	}

	return count <= EmailSendRateLimitMax, nil
}
