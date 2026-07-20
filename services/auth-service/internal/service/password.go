package service

import (
	"unicode"
	"unicode/utf8"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const minPasswordLength = 8

func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return domain.ErrWeakPassword
	}

	var hasDigit, hasLetter bool
	for _, r := range password {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsLetter(r):
			hasLetter = true
		}
	}

	if !hasDigit || !hasLetter {
		return domain.ErrWeakPassword
	}

	return nil
}
