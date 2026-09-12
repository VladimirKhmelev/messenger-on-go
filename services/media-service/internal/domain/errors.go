package domain

import "errors"

var (
	ErrNotChatMember      = errors.New("user is not a member of this chat")
	ErrMediaNotFound      = errors.New("media object not found")
	ErrUploadNotConfirmed = errors.New("upload has not been confirmed yet")
	ErrEmptyContentType   = errors.New("content type must not be empty")
	ErrInvalidSize        = errors.New("size must be greater than zero")
	ErrSizeTooLarge       = errors.New("size exceeds maximum allowed upload size")
	ErrContentTypeBlocked = errors.New("this file type is not allowed")
)
