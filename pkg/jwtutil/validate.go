package jwtutil

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrInvalidAlgorithm = errors.New("invalid signing algorithm")
)

type claims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

func ValidateAccessToken(tokenString, secret string) (string, error) {
	c := &claims{}

	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidAlgorithm
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidToken
	}

	if c.Type != "access" || c.UserID == "" {
		return "", ErrInvalidToken
	}

	return c.UserID, nil
}

func ValidateAccessTokenWithExpiry(tokenString, secret string) (userID string, expiresAt time.Time, err error) {
	c := &claims{}

	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidAlgorithm
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", time.Time{}, ErrInvalidToken
	}

	if c.Type != "access" || c.UserID == "" {
		return "", time.Time{}, ErrInvalidToken
	}

	var expiry time.Time
	if c.ExpiresAt != nil {
		expiry = c.ExpiresAt.Time
	}

	return c.UserID, expiry, nil
}
