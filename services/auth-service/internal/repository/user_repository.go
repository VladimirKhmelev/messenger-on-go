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
	SearchByTagPrefix(ctx context.Context, prefix string, limit int) ([]*domain.User, error)
}
