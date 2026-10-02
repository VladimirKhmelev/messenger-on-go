package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	email = canonicalEmail(email)

	allowed, err := s.emailSendLimiter.Allow(ctx, "reset:"+email)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrTooManyEmails
	}

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

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword, publicKey, wrappedPrivateKey, keyWrapSalt string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	if strings.TrimSpace(publicKey) == "" || strings.TrimSpace(wrappedPrivateKey) == "" || strings.TrimSpace(keyWrapSalt) == "" {
		return domain.ErrInvalidPublicKey
	}

	email, ok, err := s.passwordResets.Consume(ctx, token)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidToken
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.endSessions(ctx, user.ID); err != nil {
		return err
	}

	if err := s.users.UpdatePasswordAndKeyPair(ctx, user.ID, string(passwordHash), publicKey, wrappedPrivateKey, keyWrapSalt); err != nil {
		return err
	}

	if !user.EmailVerified {
		if err := s.users.MarkEmailVerified(ctx, user.ID); err != nil {
			return err
		}
	}

	if err := s.events.PublishUserPasswordReset(ctx, domain.UserPasswordReset{
		UserID: user.ID,
		Email:  user.Email,
		At:     time.Now(),
	}); err != nil {
		log.Printf("auth-service: failed to publish user.password_reset event for %s: %v", user.ID, err)
	}

	return nil
}
