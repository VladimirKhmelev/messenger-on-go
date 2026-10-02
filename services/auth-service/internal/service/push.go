package service

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	maxPushEndpointLength = 2048
	maxPushKeyLength      = 256
)

func (s *AuthService) SavePushSubscription(ctx context.Context, userID, endpoint, p256dhKey, authKey string) error {
	if !isPushEndpoint(endpoint) ||
		p256dhKey == "" || len(p256dhKey) > maxPushKeyLength ||
		authKey == "" || len(authKey) > maxPushKeyLength {
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

func isPushEndpoint(endpoint string) bool {
	if endpoint == "" || len(endpoint) > maxPushEndpointLength {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if !strings.Contains(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	return net.ParseIP(host) == nil
}
