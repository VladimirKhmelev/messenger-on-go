package jwtutil

import (
	"errors"

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
