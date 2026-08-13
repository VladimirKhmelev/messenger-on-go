package service

import (
	"context"
	"log"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	allowed, err := s.loginLimiter.Allow(ctx, "change-password:"+userID)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrTooManyAttempts
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return domain.ErrInvalidCredentials
	}

	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	if oldPassword == newPassword {
		return domain.ErrSamePassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.users.UpdatePasswordHash(ctx, user.ID, string(passwordHash)); err != nil {
		return err
	}

	if err := s.mailer.SendPasswordChanged(user.Email); err != nil {
		log.Printf("auth-service: failed to send password-changed notification for %s: %v", user.ID, err)
	}

	return nil
}
