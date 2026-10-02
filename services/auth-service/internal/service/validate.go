package service

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

var tagPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,19}$`)

const (
	minDisplayNameLength = 1
	maxDisplayNameLength = 40
)

func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address != trimmed {
		return "", domain.ErrInvalidEmail
	}
	return canonicalEmail(trimmed), nil
}

func ValidateEmail(email string) error {
	_, err := NormalizeEmail(email)
	return err
}

func ValidateTag(tag string) error {
	if !tagPattern.MatchString(tag) {
		return domain.ErrInvalidTag
	}
	return nil
}

func ValidateDisplayName(name string) error {
	trimmed := strings.TrimSpace(name)
	length := utf8.RuneCountInString(trimmed)
	if length < minDisplayNameLength || length > maxDisplayNameLength {
		return domain.ErrInvalidDisplayName
	}
	return nil
}
