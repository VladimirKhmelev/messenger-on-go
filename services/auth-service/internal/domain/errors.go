package domain

import "errors"

var (
	ErrInvalidEmail            = errors.New("invalid email")
	ErrInvalidTag              = errors.New("invalid tag")
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
)
