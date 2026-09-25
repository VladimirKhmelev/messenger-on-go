package service

import (
	"context"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

type MediaRepository interface {
	CreatePending(ctx context.Context, obj *domain.MediaObject) error
	Confirm(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*domain.MediaObject, error)
}

type ChatMembership interface {
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
}

type ObjectStore interface {
	PresignedPutURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	PresignedGetURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	StatObjectExists(ctx context.Context, objectKey string) (bool, error)
}
