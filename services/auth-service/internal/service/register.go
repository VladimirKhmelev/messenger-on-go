package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const oauthPasswordPlaceholder = "oauth-account-no-password"

type AuthService struct {
	users              UserRepository
	tokens             TokenIssuer
	loginLimiter       RateLimiter
	refreshBlocked     TokenBlacklist
	emailCodes         EmailVerificationStore
	emailVerifyLimiter RateLimiter
	mailer             Mailer
	passwordResets     PasswordResetStore
	github             GitHubOAuthClient
	events             EventPublisher
	passwordChanges    PasswordChangeTracker
	refreshRevoked     RefreshRevokedTracker
}

func NewAuthService(
	users UserRepository,
	tokens TokenIssuer,
	loginLimiter RateLimiter,
	refreshBlocked TokenBlacklist,
	emailCodes EmailVerificationStore,
	emailVerifyLimiter RateLimiter,
	mailer Mailer,
	passwordResets PasswordResetStore,
	github GitHubOAuthClient,
	eventPublisher EventPublisher,
	passwordChanges PasswordChangeTracker,
	refreshRevoked RefreshRevokedTracker,
) *AuthService {
	return &AuthService{
		users:              users,
		tokens:             tokens,
		loginLimiter:       loginLimiter,
		refreshBlocked:     refreshBlocked,
		emailCodes:         emailCodes,
		emailVerifyLimiter: emailVerifyLimiter,
		passwordChanges:    passwordChanges,
		refreshRevoked:     refreshRevoked,
		mailer:             mailer,
		passwordResets:     passwordResets,
		github:             github,
		events:             eventPublisher,
	}
}

func (s *AuthService) Register(ctx context.Context, email, tag, displayName, password, publicKey, wrappedPrivateKey, keyWrapSalt string) (*domain.User, error) {
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	if err := ValidateTag(tag); err != nil {
		return nil, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if strings.TrimSpace(publicKey) == "" || strings.TrimSpace(wrappedPrivateKey) == "" || strings.TrimSpace(keyWrapSalt) == "" {
		return nil, domain.ErrInvalidPublicKey
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
		ID:                uuid.NewString(),
		Email:             email,
		Tag:               tag,
		DisplayName:       strings.TrimSpace(displayName),
		PasswordHash:      string(passwordHash),
		EmailVerified:     false,
		CreatedAt:         time.Now(),
		PublicKey:         publicKey,
		WrappedPrivateKey: wrappedPrivateKey,
		KeyWrapSalt:       keyWrapSalt,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	code, err := s.emailCodes.GenerateAndStore(ctx, email)
	if err != nil {
		s.rollbackRegistration(ctx, user.ID)
		return nil, err
	}

	if err := s.mailer.SendVerificationCode(email, code); err != nil {
		s.rollbackRegistration(ctx, user.ID)
		return nil, err
	}

	if err := s.events.PublishUserRegistered(ctx, domain.UserRegistered{
		UserID:    user.ID,
		Email:     user.Email,
		Tag:       user.Tag,
		CreatedAt: user.CreatedAt,
	}); err != nil {
		log.Printf("auth-service: failed to publish user.registered event for %s: %v", user.ID, err)
	}

	return user, nil
}

func (s *AuthService) rollbackRegistration(ctx context.Context, userID string) {
	if err := s.users.Delete(ctx, userID); err != nil {
		log.Printf("auth-service: failed to roll back registration for %s after verification email failure: %v", userID, err)
	}
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
