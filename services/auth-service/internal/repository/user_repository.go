package repository

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
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
}
