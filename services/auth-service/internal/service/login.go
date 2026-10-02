package service

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

const refreshReuseGrace = 30 * time.Second

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	email = canonicalEmail(email)

	allowed, err := s.loginLimiter.Allow(ctx, "login:"+email)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrTooManyAttempts
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			metrics.FailedLoginsTotal.Inc()
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		metrics.FailedLoginsTotal.Inc()
		return nil, domain.ErrInvalidCredentials
	}

	if !user.EmailVerified {
		return nil, domain.ErrEmailNotVerified
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
	claims, err := s.tokens.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, domain.ErrInvalidToken
	}

	revokedAll, err := s.refreshRevoked.RevokedAfter(ctx, claims.UserID, claims.IssuedAt)
	if err != nil {
		return nil, err
	}
	if revokedAll {
		return nil, domain.ErrInvalidToken
	}

	ttl := time.Until(claims.ExpiresAt)
	if ttl <= 0 {
		return nil, domain.ErrInvalidToken
	}
	first, usedAt, err := s.refreshBlocked.Claim(ctx, refreshToken, ttl)
	if err != nil {
		return nil, err
	}
	if !first {
		if time.Since(usedAt) > refreshReuseGrace {
			if err := s.refreshRevoked.MarkAllRevoked(ctx, claims.UserID, domain.RefreshTokenTTL); err != nil {
				return nil, err
			}
		}
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
	claims, err := s.tokens.ParseRefreshToken(refreshToken)
	if err != nil {
		return domain.ErrInvalidToken
	}

	ttl := time.Until(claims.ExpiresAt)
	if ttl <= 0 {
		return nil
	}

	return s.refreshBlocked.Revoke(ctx, refreshToken, ttl)
}
