package service

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func (s *AuthService) DeleteAccount(ctx context.Context, userID, password string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.PasswordHash != oauthPasswordPlaceholder {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			return domain.ErrInvalidCredentials
		}
	}

	if err := s.users.DeleteAvatar(ctx, userID); err != nil {
		return err
	}

	anonymizedEmail := fmt.Sprintf("deleted-%s@deleted.local", userID)
	anonymizedTag := fmt.Sprintf("deleted_%s", userID)
	if err := s.users.Anonymize(ctx, userID, anonymizedEmail, anonymizedTag, "Удалённый пользователь"); err != nil {
		return err
	}

	if err := s.refreshRevoked.MarkAllRevoked(ctx, userID, jwtutil.RefreshTokenTTL); err != nil {
		return err
	}

	s.publishProfileUpdated(ctx, userID)
	return nil
}
