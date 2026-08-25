package service

import (
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
		case r < 0x20 || r > 0x7e:
			return domain.ErrWeakPassword
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		}
	}

	if !hasDigit || !hasLetter {
		return domain.ErrWeakPassword
	}

	return nil
}
