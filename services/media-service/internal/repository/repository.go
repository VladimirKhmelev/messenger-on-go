package repository

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

type MediaRepository interface {
	CreatePending(ctx context.Context, obj *domain.MediaObject) error
	Confirm(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*domain.MediaObject, error)
}
