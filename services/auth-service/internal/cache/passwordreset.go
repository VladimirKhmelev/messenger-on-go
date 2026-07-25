package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	passwordResetKeyPrefix = "auth:password-reset:"
	PasswordResetTokenTTL  = 30 * time.Minute
	passwordResetTokenSize = 32
)

type PasswordResetStore struct {
	client *redis.Client
}

func NewPasswordResetStore(client *redis.Client) *PasswordResetStore {
	return &PasswordResetStore{client: client}
}

func (s *PasswordResetStore) GenerateAndStore(ctx context.Context, email string) (string, error) {
	token, err := generateResetToken()
	if err != nil {
		return "", err
	}

	if err := s.client.Set(ctx, passwordResetKeyPrefix+hashToken(token), email, PasswordResetTokenTTL).Err(); err != nil {
		return "", err
	}

	return token, nil
}

func (s *PasswordResetStore) Consume(ctx context.Context, token string) (string, bool, error) {
	key := passwordResetKeyPrefix + hashToken(token)

	email, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return "", false, err
	}

	return email, true, nil
}

func generateResetToken() (string, error) {
	buf := make([]byte, passwordResetTokenSize)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
