package service

import (
	"context"
	"errors"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func (s *AuthService) UpdateTag(ctx context.Context, userID, tag string) error {
	if err := ValidateTag(tag); err != nil {
		return err
	}

	existing, err := s.users.GetByTag(ctx, tag)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return err
	}
	if existing != nil && existing.ID != userID {
		return domain.ErrTagTaken
	}

	return s.users.UpdateTag(ctx, userID, tag)
}

func (s *AuthService) CheckTagAvailable(ctx context.Context, tag string) (available bool, suggested string, err error) {
	if err := ValidateTag(tag); err != nil {
		return false, "", err
	}

	taken, err := s.users.ExistsByTag(ctx, tag)
	if err != nil {
		return false, "", err
	}
	if !taken {
		return true, "", nil
	}

	suggested, err = s.generateUniqueTag(ctx, tag)
	if err != nil {
		return false, "", err
	}
	return false, suggested, nil
}
