package jwtutil

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestIssueAndParseAccessToken(t *testing.T) {
	issuer := NewIssuer("stepok")

	token, err := issuer.IssueAccessToken("user-1")
	if err != nil {
		t.Fatalf("IssueAccessToken() unexpected error: %v", err)
	}

	claims, err := issuer.Parse(token, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if claims.UserID != "user-1" {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, "user-1")
	}
	if claims.Type != TokenTypeAccess {
		t.Errorf("claims.Type = %q, want %q", claims.Type, TokenTypeAccess)
	}
}

func TestIssueAndParseRefreshToken(t *testing.T) {
	issuer := NewIssuer("stepok")

	token, err := issuer.IssueRefreshToken("user-1")
	if err != nil {
		t.Fatalf("IssueRefreshToken() unexpected error: %v", err)
	}

	claims, err := issuer.Parse(token, TokenTypeRefresh)
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if claims.Type != TokenTypeRefresh {
		t.Errorf("claims.Type = %q, want %q", claims.Type, TokenTypeRefresh)
	}
}

func TestParse_WrongTokenType(t *testing.T) {
	issuer := NewIssuer("stepok")

	token, err := issuer.IssueAccessToken("user-1")
	if err != nil {
		t.Fatalf("IssueAccessToken() unexpected error: %v", err)
	}

	_, err = issuer.Parse(token, TokenTypeRefresh)
	if err != domain.ErrInvalidToken {
		t.Errorf("Parse() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	issuer := NewIssuer("stepok")
	otherIssuer := NewIssuer("other-stepok")

	token, err := issuer.IssueAccessToken("user-1")
	if err != nil {
		t.Fatalf("IssueAccessToken() unexpected error: %v", err)
	}

	_, err = otherIssuer.Parse(token, TokenTypeAccess)
	if err != domain.ErrInvalidToken {
		t.Errorf("Parse() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestParse_ExpiredToken(t *testing.T) {
	issuer := NewIssuer("stepok")

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
		UserID: "user-1",
		Type:   TokenTypeAccess,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(issuer.secret)
	if err != nil {
		t.Fatalf("SignedString() unexpected error: %v", err)
	}

	_, err = issuer.Parse(signed, TokenTypeAccess)
	if err != domain.ErrInvalidToken {
		t.Errorf("Parse() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}

func TestParse_MalformedToken(t *testing.T) {
	issuer := NewIssuer("stepok")

	_, err := issuer.Parse("not-a-valid-token", TokenTypeAccess)
	if err != domain.ErrInvalidToken {
		t.Errorf("Parse() error = %v, want %v", err, domain.ErrInvalidToken)
	}
}
