package service

import (
	"context"
	"errors"
	"log"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/events"
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

	if err := s.users.UpdateTag(ctx, userID, tag); err != nil {
		return err
	}

	s.publishProfileUpdated(ctx, userID)
	return nil
}

func (s *AuthService) publishProfileUpdated(ctx context.Context, userID string) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		log.Printf("auth-service: failed to load user %s for profile_updated event: %v", userID, err)
		return
	}

	if err := s.events.PublishUserProfileUpdated(ctx, events.UserProfileUpdated{
		UserID:      user.ID,
		Tag:         user.Tag,
		DisplayName: user.DisplayName,
	}); err != nil {
		log.Printf("auth-service: failed to publish user.profile_updated event for %s: %v", userID, err)
	}
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
