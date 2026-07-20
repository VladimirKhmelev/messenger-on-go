package service

import (
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"valid", "abcd1234", nil},
		{"too short", "abc123", domain.ErrWeakPassword},
		{"no digit", "abcdefgh", domain.ErrWeakPassword},
		{"no letter", "52675267", domain.ErrWeakPassword},
		{"empty", "", domain.ErrWeakPassword},
		{"cyrillic too short (byte length would pass)", "аааа1", domain.ErrWeakPassword},
		{"cyrillic valid", "пароль12", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidatePassword(%q) = %v, want %v", tc.password, err, tc.wantErr)
			}
		})
	}
}
