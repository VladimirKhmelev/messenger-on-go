package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

const maxTagGenerationAttempts = 5

func (s *AuthService) LoginWithGitHub(ctx context.Context, code string) (*TokenPair, error) {
	profile, err := s.github.FetchProfile(code)
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByEmail(ctx, profile.Email)
	if err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			return nil, err
		}

		user, err = s.createUserFromGitHub(ctx, profile)
		if err != nil {
			return nil, err
		}
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

func (s *AuthService) createUserFromGitHub(ctx context.Context, profile *oauth.GitHubProfile) (*domain.User, error) {
	tag, err := s.generateUniqueTag(ctx)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:            uuid.NewString(),
		Email:         profile.Email,
		Tag:           tag,
		PasswordHash:  oauthPasswordPlaceholder,
		EmailVerified: true,
		CreatedAt:     time.Now(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) generateUniqueTag(ctx context.Context) (string, error) {
	for i := 0; i < maxTagGenerationAttempts; i++ {
		tag, err := randomTag()
		if err != nil {
			return "", err
		}

		taken, err := s.users.ExistsByTag(ctx, tag)
		if err != nil {
			return "", err
		}
		if !taken {
			return tag, nil
		}
	}

	return "", errors.New("failed to generate a unique tag")
}

func randomTag() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("id%08d", n.Int64()), nil
}
