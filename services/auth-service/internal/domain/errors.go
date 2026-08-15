package domain

import "errors"

var (
	ErrInvalidEmail            = errors.New("invalid email")
	ErrInvalidTag              = errors.New("invalid tag")
	ErrInvalidDisplayName      = errors.New("invalid display name")
	ErrWeakPassword            = errors.New("password does not meet complexity requirements")
	ErrEmailTaken              = errors.New("email already registered")
	ErrTagTaken                = errors.New("tag already taken")
	ErrUserNotFound            = errors.New("user not found")
	ErrInvalidToken            = errors.New("invalid or expired token")
	ErrInvalidCredentials      = errors.New("invalid email or password")
	ErrSearchQueryTooShort     = errors.New("search query must be at least 3 characters")
	ErrTooManyAttempts         = errors.New("too many login attempts, try again later")
	ErrInvalidVerificationCode = errors.New("invalid or expired verification code")
	ErrEmailNotVerified        = errors.New("email not verified")
	ErrOAuthNoVerifiedEmail    = errors.New("oauth provider account has no verified email")
	ErrSamePassword            = errors.New("new password must be different from the current password")
	ErrAvatarNotFound          = errors.New("avatar not found")
	ErrInvalidAvatarType       = errors.New("avatar must be a JPEG, PNG, GIF, or WebP image")
	ErrAvatarTooLarge          = errors.New("avatar must be smaller than 2MB")
)
