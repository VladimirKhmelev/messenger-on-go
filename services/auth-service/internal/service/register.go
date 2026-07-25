package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/repository"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type TokenBlacklist interface {
	Revoke(ctx context.Context, token string, ttl time.Duration) error
	IsRevoked(ctx context.Context, token string) (bool, error)
}

type EmailVerificationStore interface {
	GenerateAndStore(ctx context.Context, email string) (string, error)
	Verify(ctx context.Context, email, code string) (bool, error)
}

type Mailer interface {
	SendVerificationCode(to, code string) error
	SendPasswordResetToken(to, token string) error
}

type PasswordResetStore interface {
	GenerateAndStore(ctx context.Context, email string) (string, error)
	Consume(ctx context.Context, token string) (email string, ok bool, err error)
}

type AuthService struct {
	users              repository.UserRepository
	tokens             *jwtutil.Issuer
	loginLimiter       RateLimiter
	refreshBlocked     TokenBlacklist
	emailCodes         EmailVerificationStore
	emailVerifyLimiter RateLimiter
	mailer             Mailer
	passwordResets     PasswordResetStore
}

func NewAuthService(
	users repository.UserRepository,
	tokens *jwtutil.Issuer,
	loginLimiter RateLimiter,
	refreshBlocked TokenBlacklist,
	emailCodes EmailVerificationStore,
	emailVerifyLimiter RateLimiter,
	mailer Mailer,
	passwordResets PasswordResetStore,
) *AuthService {
	return &AuthService{
		users:              users,
		tokens:             tokens,
		loginLimiter:       loginLimiter,
		refreshBlocked:     refreshBlocked,
		emailCodes:         emailCodes,
		emailVerifyLimiter: emailVerifyLimiter,
		mailer:             mailer,
		passwordResets:     passwordResets,
	}
}

func (s *AuthService) Register(ctx context.Context, email, tag, password string) (*domain.User, error) {
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	if err := ValidateTag(tag); err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	emailTaken, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if emailTaken {
		return nil, domain.ErrEmailTaken
	}

	tagTaken, err := s.users.ExistsByTag(ctx, tag)
	if err != nil {
		return nil, err
	}
	if tagTaken {
		return nil, domain.ErrTagTaken
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:            uuid.NewString(),
		Email:         email,
		Tag:           tag,
		PasswordHash:  string(passwordHash),
		EmailVerified: false,
		CreatedAt:     time.Now(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	code, err := s.emailCodes.GenerateAndStore(ctx, email)
	if err != nil {
		return nil, err
	}

	if err := s.mailer.SendVerificationCode(email, code); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, email, code string) error {
	allowed, err := s.emailVerifyLimiter.Allow(ctx, email)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrTooManyAttempts
	}

	ok, err := s.emailCodes.Verify(ctx, email, code)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidVerificationCode
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}

	return s.users.MarkEmailVerified(ctx, user.ID)
}
