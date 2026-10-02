package service

import (
	"context"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword, wrappedPrivateKey, keyWrapSalt string) (string, error) {
	allowed, err := s.loginLimiter.Allow(ctx, "change-password:"+userID)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", domain.ErrTooManyAttempts
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	if err := ValidatePassword(newPassword); err != nil {
		return "", err
	}

	if oldPassword == newPassword {
		return "", domain.ErrSamePassword
	}

	if strings.TrimSpace(wrappedPrivateKey) == "" || strings.TrimSpace(keyWrapSalt) == "" {
		return "", domain.ErrInvalidPublicKey
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	if err := s.endSessions(ctx, user.ID); err != nil {
		return "", err
	}

	if err := s.users.UpdatePasswordAndWrappedKey(ctx, user.ID, string(passwordHash), wrappedPrivateKey, keyWrapSalt); err != nil {
		return "", err
	}

	refreshToken, err := s.tokens.IssueRefreshToken(user.ID)
	if err != nil {
		return "", err
	}

	if err := s.mailer.SendPasswordChanged(user.Email); err != nil {
		log.Printf("auth-service: failed to send password-changed notification for %s: %v", user.ID, err)
	}

	return refreshToken, nil
}

func (s *AuthService) IsAccessTokenStale(ctx context.Context, userID string, issuedAt time.Time) (bool, error) {
	return s.passwordChanges.ChangedAfter(ctx, userID, issuedAt)
}
