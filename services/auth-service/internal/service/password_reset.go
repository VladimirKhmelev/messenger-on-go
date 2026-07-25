package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil
		}
		return err
	}

	token, err := s.passwordResets.GenerateAndStore(ctx, user.Email)
	if err != nil {
		return err
	}

	return s.mailer.SendPasswordResetToken(user.Email, token)
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	email, ok, err := s.passwordResets.Consume(ctx, token)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidToken
	}

	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.users.UpdatePasswordHash(ctx, user.ID, string(passwordHash))
}
