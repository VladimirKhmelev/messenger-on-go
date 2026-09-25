package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

const maxTagGenerationAttempts = 5

type GitHubLoginResult struct {
	Tokens    *TokenPair
	IsNewUser bool
}

func (s *AuthService) LoginWithGitHub(ctx context.Context, code, publicKey, wrappedPrivateKey, keyWrapSalt string) (*GitHubLoginResult, error) {
	profile, err := s.github.FetchProfile(code)
	if err != nil {
		return nil, err
	}

	isNewUser := false
	user, err := s.users.GetByEmail(ctx, profile.Email)
	if err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			return nil, err
		}

		if strings.TrimSpace(publicKey) == "" || strings.TrimSpace(wrappedPrivateKey) == "" || strings.TrimSpace(keyWrapSalt) == "" {
			return nil, domain.ErrInvalidPublicKey
		}

		user, err = s.createUserFromGitHub(ctx, profile, publicKey, wrappedPrivateKey, keyWrapSalt)
		if err != nil {
			return nil, err
		}
		isNewUser = true
	}

	accessToken, err := s.tokens.IssueAccessToken(user.ID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.tokens.IssueRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &GitHubLoginResult{
		Tokens:    &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken},
		IsNewUser: isNewUser,
	}, nil
}

func (s *AuthService) createUserFromGitHub(ctx context.Context, profile *oauth.GitHubProfile, publicKey, wrappedPrivateKey, keyWrapSalt string) (*domain.User, error) {
	tag, err := s.generateUniqueTag(ctx, "id")
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:                uuid.NewString(),
		Email:             profile.Email,
		Tag:               tag,
		DisplayName:       profile.Login,
		PasswordHash:      oauthPasswordPlaceholder,
		EmailVerified:     true,
		CreatedAt:         time.Now(),
		PublicKey:         publicKey,
		WrappedPrivateKey: wrappedPrivateKey,
		KeyWrapSalt:       keyWrapSalt,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	if err := s.events.PublishUserOAuthLinked(ctx, events.UserOAuthLinked{
		UserID:   user.ID,
		Email:    user.Email,
		Provider: "github",
		At:       time.Now(),
	}); err != nil {
		log.Printf("auth-service: failed to publish user.oauth_linked event for %s: %v", user.ID, err)
	}

	return user, nil
}

func (s *AuthService) generateUniqueTag(ctx context.Context, base string) (string, error) {
	for i := 0; i < maxTagGenerationAttempts; i++ {
		tag, err := randomTag(base)
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

func randomTag(base string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", err
	}
	suffix := fmt.Sprintf("%08d", n.Int64())

	const maxTagLength = 20
	if len(base)+len(suffix) > maxTagLength {
		base = base[:maxTagLength-len(suffix)]
	}

	return base + suffix, nil
}
