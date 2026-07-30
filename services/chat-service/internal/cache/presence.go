package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	onlineKeyPrefix = "chat:online:"
	onlineTTL       = 30 * time.Second
)

type PresenceStore struct {
	client *redis.Client
}

func NewPresenceStore(client *redis.Client) *PresenceStore {
	return &PresenceStore{client: client}
}

func (s *PresenceStore) SetOnline(ctx context.Context, userID string) error {
	return s.client.Set(ctx, onlineKeyPrefix+userID, "1", onlineTTL).Err()
}

func (s *PresenceStore) SetOffline(ctx context.Context, userID string) error {
	return s.client.Del(ctx, onlineKeyPrefix+userID).Err()
}

func (s *PresenceStore) IsOnline(ctx context.Context, userID string) (bool, error) {
	exists, err := s.client.Exists(ctx, onlineKeyPrefix+userID).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
