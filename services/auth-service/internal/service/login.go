package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	accessToken, err := s.tokens.IssueAccessToken(user.ID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.tokens.IssueRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
