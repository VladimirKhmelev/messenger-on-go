package service

import (
	"context"
	"strings"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) GetPublicKey(ctx context.Context, userID string) (string, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user.PublicKey == "" {
		return "", domain.ErrPublicKeyNotSet
	}
	return user.PublicKey, nil
}

func (s *AuthService) GetWrappedPrivateKey(ctx context.Context, userID string) (wrappedPrivateKey, keyWrapSalt string, err error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if user.WrappedPrivateKey == "" || user.KeyWrapSalt == "" {
		return "", "", domain.ErrPublicKeyNotSet
	}
	return user.WrappedPrivateKey, user.KeyWrapSalt, nil
}

func (s *AuthService) RewrapPrivateKey(ctx context.Context, userID, wrappedPrivateKey, keyWrapSalt string) error {
	if strings.TrimSpace(wrappedPrivateKey) == "" || strings.TrimSpace(keyWrapSalt) == "" {
		return domain.ErrInvalidPublicKey
	}

	return s.users.UpdateWrappedPrivateKey(ctx, userID, wrappedPrivateKey, keyWrapSalt)
}
