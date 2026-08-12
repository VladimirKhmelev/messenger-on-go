package service

import (
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func TestValidateEmail(t *testing.T) {
	cases := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid", "user@example.com", nil},
		{"missing at", "userexample.com", domain.ErrInvalidEmail},
		{"missing domain", "user@", domain.ErrInvalidEmail},
		{"empty", "", domain.ErrInvalidEmail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tc.email, err, tc.wantErr)
			}
		})
	}
}

func TestValidateTag(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		wantErr error
	}{
		{"valid", "john_doe1", nil},
		{"too short", "ab", domain.ErrInvalidTag},
		{"too long", "abcdefghijklmnopqrstu", domain.ErrInvalidTag},
		{"uppercase", "JohnDoe", domain.ErrInvalidTag},
		{"invalid char", "john-doe", domain.ErrInvalidTag},
		{"empty", "", domain.ErrInvalidTag},
		{"starts with digit", "1john", domain.ErrInvalidTag},
		{"starts with underscore", "_john", domain.ErrInvalidTag},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTag(tc.tag)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateTag(%q) = %v, want %v", tc.tag, err, tc.wantErr)
			}
		})
	}
}

func TestValidateDisplayName(t *testing.T) {
	cases := []struct {
		name    string
		display string
		wantErr error
	}{
		{"valid ascii", "John", nil},
		{"valid cyrillic", "Степаныч", nil},
		{"valid emoji", "🔥 Stepok 🔥", nil},
		{"single char", "J", nil},
		{"empty", "", domain.ErrInvalidDisplayName},
		{"whitespace only", "   ", domain.ErrInvalidDisplayName},
		{"too long", strings40plus1(), domain.ErrInvalidDisplayName},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDisplayName(tc.display)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateDisplayName(%q) = %v, want %v", tc.display, err, tc.wantErr)
			}
		})
	}
}

func strings40plus1() string {
	name := ""
	for i := 0; i < 41; i++ {
		name += "a"
	}
	return name
}
