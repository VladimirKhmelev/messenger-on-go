package service

import (
	"net/mail"
	"regexp"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

var tagPattern = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)

func ValidateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return domain.ErrInvalidEmail
	}
	return nil
}

func ValidateTag(tag string) error {
	if !tagPattern.MatchString(tag) {
		return domain.ErrInvalidTag
	}
	return nil
}
