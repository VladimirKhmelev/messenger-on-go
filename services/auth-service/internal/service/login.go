package service

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	allowed, err := s.loginLimiter.Allow(ctx, email)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrTooManyAttempts
	}

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

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, jwtutil.TokenTypeRefresh)
	if err != nil {
		return nil, domain.ErrInvalidToken
	}

	revoked, err := s.refreshBlocked.IsRevoked(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, domain.ErrInvalidToken
	}

	accessToken, err := s.tokens.IssueAccessToken(claims.UserID)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.tokens.IssueRefreshToken(claims.UserID)
	if err != nil {
		return nil, err
	}

	return &TokenPair{AccessToken: accessToken, RefreshToken: newRefreshToken}, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokens.Parse(refreshToken, jwtutil.TokenTypeRefresh)
	if err != nil {
		return domain.ErrInvalidToken
	}

	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return nil
	}

	return s.refreshBlocked.Revoke(ctx, refreshToken, ttl)
}
