package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	onlineKeyPrefix   = "chat:online:"
	lastSeenKeyPrefix = "chat:last_seen:"
	onlineTTL         = 30 * time.Second

	typingKeyPrefix = "chat:typing:"
	typingTTL       = 3 * time.Second
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
	if err := s.client.Set(ctx, lastSeenKeyPrefix+userID, time.Now().Unix(), 0).Err(); err != nil {
		return err
	}
	return s.client.Del(ctx, onlineKeyPrefix+userID).Err()
}

func (s *PresenceStore) IsOnline(ctx context.Context, userID string) (bool, error) {
	exists, err := s.client.Exists(ctx, onlineKeyPrefix+userID).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

func (s *PresenceStore) LastSeen(ctx context.Context, userID string) (int64, error) {
	val, err := s.client.Get(ctx, lastSeenKeyPrefix+userID).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (s *PresenceStore) SetTyping(ctx context.Context, chatID, userID string) error {
	return s.client.Set(ctx, typingKeyPrefix+chatID+":"+userID, "1", typingTTL).Err()
}

func (s *PresenceStore) IsTyping(ctx context.Context, chatID, userID string) (bool, error) {
	exists, err := s.client.Exists(ctx, typingKeyPrefix+chatID+":"+userID).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
