package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) SavePushSubscription(ctx context.Context, userID, endpoint, p256dhKey, authKey string) error {

	if endpoint == "" || p256dhKey == "" || authKey == "" {
		return domain.ErrInvalidPushSubscription
	}

	return s.users.UpsertPushSubscription(ctx, &domain.PushSubscription{
		ID:        uuid.NewString(),
		UserID:    userID,
		Endpoint:  endpoint,
		P256dhKey: p256dhKey,
		AuthKey:   authKey,
		CreatedAt: time.Now(),
	})
}

func (s *AuthService) DeletePushSubscription(ctx context.Context, userID, endpoint string) error {
	return s.users.DeletePushSubscription(ctx, userID, endpoint)
}

func (s *AuthService) ListPushSubscriptions(ctx context.Context, userID string) ([]*domain.PushSubscription, error) {
	return s.users.ListPushSubscriptions(ctx, userID)
}
