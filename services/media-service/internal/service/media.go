package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/repository"
)

const (
	MaxUploadSizeBytes = 50 * 1024 * 1024 // 50MB

	uploadURLTTL   = 15 * time.Minute
	downloadURLTTL = 15 * time.Minute
)

type ChatMembership interface {
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
}

type ObjectStore interface {
	PresignedPutURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	PresignedGetURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	StatObjectExists(ctx context.Context, objectKey string) (bool, error)
}

type MediaService struct {
	media   repository.MediaRepository
	store   ObjectStore
	members ChatMembership
}

func NewMediaService(media repository.MediaRepository, store ObjectStore, members ChatMembership) *MediaService {
	return &MediaService{media: media, store: store, members: members}
}

type UploadTicket struct {
	UploadID      string
	UploadURL     string
	ExpiresAtUnix int64
}

func (s *MediaService) RequestUpload(ctx context.Context, chatID, requesterID, contentType string, sizeBytes int64) (*UploadTicket, error) {
	if contentType == "" {
		return nil, domain.ErrEmptyContentType
	}
	if sizeBytes <= 0 {
		return nil, domain.ErrInvalidSize
	}
	if sizeBytes > MaxUploadSizeBytes {
		return nil, domain.ErrSizeTooLarge
	}

	isMember, err := s.members.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	id := uuid.NewString()
	objectKey := "chats/" + chatID + "/" + id

	if err := s.media.CreatePending(ctx, &domain.MediaObject{
		ID:          id,
		ChatID:      chatID,
		UploaderID:  requesterID,
		ObjectKey:   objectKey,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		CreatedAt:   time.Now(),
	}); err != nil {
		return nil, err
	}

	uploadURL, err := s.store.PresignedPutURL(ctx, objectKey, uploadURLTTL)
	if err != nil {
		return nil, err
	}

	return &UploadTicket{
		UploadID:      id,
		UploadURL:     uploadURL,
		ExpiresAtUnix: time.Now().Add(uploadURLTTL).Unix(),
	}, nil
}

func (s *MediaService) ConfirmUpload(ctx context.Context, uploadID, requesterID string) (string, error) {
	obj, err := s.media.Get(ctx, uploadID)
	if err != nil {
		return "", err
	}
	if obj.UploaderID != requesterID {
		return "", domain.ErrMediaNotFound
	}

	exists, err := s.store.StatObjectExists(ctx, obj.ObjectKey)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", domain.ErrUploadNotConfirmed
	}

	if err := s.media.Confirm(ctx, uploadID); err != nil {
		return "", err
	}

	return obj.ID, nil
}

type DownloadTicket struct {
	DownloadURL   string
	ExpiresAtUnix int64
}

func (s *MediaService) GetDownloadURL(ctx context.Context, mediaID, requesterID string) (*DownloadTicket, error) {
	obj, err := s.media.Get(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if !obj.Confirmed {
		return nil, domain.ErrUploadNotConfirmed
	}

	isMember, err := s.members.IsMember(ctx, obj.ChatID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrNotChatMember
	}

	downloadURL, err := s.store.PresignedGetURL(ctx, obj.ObjectKey, downloadURLTTL)
	if err != nil {
		return nil, err
	}

	return &DownloadTicket{
		DownloadURL:   downloadURL,
		ExpiresAtUnix: time.Now().Add(downloadURLTTL).Unix(),
	}, nil
}
