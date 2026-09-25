package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, userID string) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByTag(ctx context.Context, tag string) (bool, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByTag(ctx context.Context, tag string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	SearchByTagPrefix(ctx context.Context, prefix string, limit int) ([]*domain.User, error)
	MarkEmailVerified(ctx context.Context, userID string) error
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
	UpdateTag(ctx context.Context, userID, tag string) error
	UpdateDisplayName(ctx context.Context, userID, displayName string) error
	UpdatePublicKey(ctx context.Context, userID, publicKey string) error
	UpdateWrappedPrivateKey(ctx context.Context, userID, wrappedPrivateKey, keyWrapSalt string) error
	UpsertAvatar(ctx context.Context, avatar *domain.Avatar) error
	GetAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
	DeleteAvatar(ctx context.Context, userID string) error
	Anonymize(ctx context.Context, userID, anonymizedEmail, anonymizedTag, anonymizedDisplayName string) error
	UpsertPushSubscription(ctx context.Context, sub *domain.PushSubscription) error
	DeletePushSubscription(ctx context.Context, userID, endpoint string) error
	ListPushSubscriptions(ctx context.Context, userID string) ([]*domain.PushSubscription, error)
}

type TokenIssuer interface {
	IssueAccessToken(userID string) (string, error)
	IssueRefreshToken(userID string) (string, error)
	ParseRefreshToken(token string) (*domain.TokenClaims, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type TokenBlacklist interface {
	Revoke(ctx context.Context, token string, ttl time.Duration) error
	IsRevoked(ctx context.Context, token string) (bool, error)
}

type PasswordChangeTracker interface {
	MarkChanged(ctx context.Context, userID string, ttl time.Duration) error
	ChangedAfter(ctx context.Context, userID string, issuedAt time.Time) (bool, error)
}

type RefreshRevokedTracker interface {
	MarkAllRevoked(ctx context.Context, userID string, ttl time.Duration) error
	RevokedAfter(ctx context.Context, userID string, issuedAt time.Time) (bool, error)
}

type EmailVerificationStore interface {
	GenerateAndStore(ctx context.Context, email string) (string, error)
	Verify(ctx context.Context, email, code string) (bool, error)
}

type Mailer interface {
	SendVerificationCode(to, code string) error
	SendPasswordResetToken(to, token string) error
	SendPasswordChanged(to string) error
}

type PasswordResetStore interface {
	GenerateAndStore(ctx context.Context, email string) (string, error)
	Consume(ctx context.Context, token string) (email string, ok bool, err error)
}

type GitHubOAuthClient interface {
	FetchProfile(code string) (*domain.GitHubProfile, error)
}

type EventPublisher interface {
	PublishUserRegistered(ctx context.Context, event domain.UserRegistered) error
	PublishUserPasswordReset(ctx context.Context, event domain.UserPasswordReset) error
	PublishUserOAuthLinked(ctx context.Context, event domain.UserOAuthLinked) error
	PublishUserProfileUpdated(ctx context.Context, event domain.UserProfileUpdated) error
}
