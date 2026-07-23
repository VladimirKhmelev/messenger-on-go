package cache

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	emailVerificationKeyPrefix = "auth:email-verification:"
	EmailVerificationCodeTTL   = 10 * time.Minute
)

type EmailVerificationStore struct {
	client *redis.Client
}

func NewEmailVerificationStore(client *redis.Client) *EmailVerificationStore {
	return &EmailVerificationStore{client: client}
}

func (s *EmailVerificationStore) GenerateAndStore(ctx context.Context, email string) (string, error) {
	code, err := generateCode()
	if err != nil {
		return "", err
	}

	if err := s.client.Set(ctx, emailVerificationKeyPrefix+email, code, EmailVerificationCodeTTL).Err(); err != nil {
		return "", err
	}

	return code, nil
}

func (s *EmailVerificationStore) Verify(ctx context.Context, email, code string) (bool, error) {
	key := emailVerificationKeyPrefix + email

	stored, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if stored != code {
		return false, nil
	}

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return false, err
	}

	return true, nil
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
