package service

import (
	"context"
	"strings"
)

func (s *AuthService) UpdateDisplayName(ctx context.Context, userID, displayName string) error {
	trimmed := strings.TrimSpace(displayName)
	if err := ValidateDisplayName(trimmed); err != nil {
		return err
	}

	if err := s.users.UpdateDisplayName(ctx, userID, trimmed); err != nil {
		return err
	}

	s.publishProfileUpdated(ctx, userID)
	return nil
}
