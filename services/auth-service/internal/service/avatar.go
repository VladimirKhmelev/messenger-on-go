package service

import (
	"context"
	"net/http"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const MaxAvatarSizeBytes = 2 * 1024 * 1024 // 2MB

var allowedAvatarContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

func (s *AuthService) UploadAvatar(ctx context.Context, userID string, data []byte) error {
	if len(data) > MaxAvatarSizeBytes {
		return domain.ErrAvatarTooLarge
	}

	contentType := http.DetectContentType(data)
	if !allowedAvatarContentTypes[contentType] {
		return domain.ErrInvalidAvatarType
	}

	if err := s.users.UpsertAvatar(ctx, &domain.Avatar{
		UserID:      userID,
		Data:        data,
		ContentType: contentType,
		UpdatedAt:   time.Now(),
	}); err != nil {
		return err
	}

	s.publishProfileUpdated(ctx, userID)
	return nil
}

func (s *AuthService) GetAvatar(ctx context.Context, userID string) (*domain.Avatar, error) {
	return s.users.GetAvatar(ctx, userID)
}

func (s *AuthService) DeleteAvatar(ctx context.Context, userID string) error {
	if err := s.users.DeleteAvatar(ctx, userID); err != nil {
		return err
	}

	s.publishProfileUpdated(ctx, userID)
	return nil
}
