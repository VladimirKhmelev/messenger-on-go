package service

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) GetPublicKey(ctx context.Context, userID string) (string, error) {
	if !isUserID(userID) {
		return "", domain.ErrUserNotFound
	}
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
