package domain

import "time"

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

type TokenClaims struct {
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type GitHubProfile struct {
	ID    int64
	Login string
	Email string
}
