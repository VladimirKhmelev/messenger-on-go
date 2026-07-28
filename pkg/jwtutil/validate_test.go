package jwtutil

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(t *testing.T, secret string, method jwt.SigningMethod, c claims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, c)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func validClaims(userID string) claims {
	now := time.Now()
	return claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		UserID: userID,
		Type:   "access",
	}
}

func TestValidateAccessToken_Success(t *testing.T) {
	token := signToken(t, "secret", jwt.SigningMethodHS256, validClaims("user-1"))

	userID, err := ValidateAccessToken(token, "secret")
	if err != nil {
		t.Fatalf("ValidateAccessToken() unexpected error: %v", err)
	}
	if userID != "user-1" {
		t.Errorf("ValidateAccessToken() userID = %q, want %q", userID, "user-1")
	}
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	token := signToken(t, "secret", jwt.SigningMethodHS256, validClaims("user-1"))

	_, err := ValidateAccessToken(token, "wrong-secret")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	c := validClaims("user-1")
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
	token := signToken(t, "secret", jwt.SigningMethodHS256, c)

	_, err := ValidateAccessToken(token, "secret")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateAccessToken_WrongType(t *testing.T) {
	c := validClaims("user-1")
	c.Type = "refresh"
	token := signToken(t, "secret", jwt.SigningMethodHS256, c)

	_, err := ValidateAccessToken(token, "secret")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateAccessToken_MissingUserID(t *testing.T) {
	token := signToken(t, "secret", jwt.SigningMethodHS256, validClaims(""))

	_, err := ValidateAccessToken(token, "secret")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateAccessToken_MalformedToken(t *testing.T) {
	_, err := ValidateAccessToken("not-a-jwt", "secret")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken() error = %v, want %v", err, ErrInvalidToken)
	}
}
