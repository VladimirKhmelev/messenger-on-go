package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/repository"
)

type AuthService struct {
	users repository.UserRepository
}

func NewAuthService(users repository.UserRepository) *AuthService {
	return &AuthService{users: users}
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
		ID:           uuid.NewString(),
		Email:        email,
		Tag:          tag,
		PasswordHash: string(passwordHash),
		CreatedAt:    time.Now(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
