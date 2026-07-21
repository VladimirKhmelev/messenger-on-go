package jwtutil

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID string    `json:"user_id"`
	Type   TokenType `json:"type"`
}

type Issuer struct {
	secret []byte
}

func NewIssuer(secret string) *Issuer {
	return &Issuer{secret: []byte(secret)}
}

func (i *Issuer) IssueAccessToken(userID string) (string, error) {
	return i.issue(userID, TokenTypeAccess, AccessTokenTTL)
}

func (i *Issuer) IssueRefreshToken(userID string) (string, error) {
	return i.issue(userID, TokenTypeRefresh, RefreshTokenTTL)
}

func (i *Issuer) issue(userID string, tokenType TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		UserID: userID,
		Type:   tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(i.secret)
}

func (i *Issuer) Parse(tokenString string, wantType TokenType) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, domain.ErrInvalidToken
		}
		return i.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, domain.ErrInvalidToken
	}

	if claims.Type != wantType {
		return nil, domain.ErrInvalidToken
	}

	return claims, nil
}
