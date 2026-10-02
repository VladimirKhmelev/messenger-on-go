package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) endSessions(ctx context.Context, userID string) error {
	if err := s.refreshRevoked.MarkAllRevoked(ctx, userID, domain.RefreshTokenTTL); err != nil {
		return err
	}
	return s.passwordChanges.MarkChanged(ctx, userID, domain.AccessTokenTTL)
}

func canonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUserID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}
