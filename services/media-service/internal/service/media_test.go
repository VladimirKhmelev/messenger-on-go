package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

type fakeMediaRepository struct {
	objects map[string]*domain.MediaObject
}

func newFakeMediaRepository() *fakeMediaRepository {
	return &fakeMediaRepository{objects: make(map[string]*domain.MediaObject)}
}

func (r *fakeMediaRepository) CreatePending(_ context.Context, obj *domain.MediaObject) error {
	cp := *obj
	r.objects[obj.ID] = &cp
	return nil
}

func (r *fakeMediaRepository) Confirm(_ context.Context, id string) error {
	obj, ok := r.objects[id]
	if !ok {
		return domain.ErrMediaNotFound
	}
	obj.Confirmed = true
	return nil
}

func (r *fakeMediaRepository) Get(_ context.Context, id string) (*domain.MediaObject, error) {
	obj, ok := r.objects[id]
	if !ok {
		return nil, domain.ErrMediaNotFound
	}
	cp := *obj
	return &cp, nil
}

type fakeObjectStore struct {
	existing map[string]bool
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{existing: make(map[string]bool)}
}

func (s *fakeObjectStore) PresignedPutURL(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return "https://example.test/put/" + objectKey, nil
}

func (s *fakeObjectStore) PresignedGetURL(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return "https://example.test/get/" + objectKey, nil
}

func (s *fakeObjectStore) StatObjectExists(_ context.Context, objectKey string) (bool, error) {
	return s.existing[objectKey], nil
}

type fakeChatMembership struct {
	members map[string]map[string]bool
}

func newFakeChatMembership(chatID string, userIDs ...string) *fakeChatMembership {
	m := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		m[id] = true
	}
	return &fakeChatMembership{members: map[string]map[string]bool{chatID: m}}
}

func (c *fakeChatMembership) IsMember(_ context.Context, chatID, userID string) (bool, error) {
	return c.members[chatID][userID], nil
}

func TestMediaService_RequestUpload_Success(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}
	if ticket.UploadID == "" {
		t.Error("RequestUpload() returned empty UploadID")
	}
	if ticket.UploadURL == "" {
		t.Error("RequestUpload() returned empty UploadURL")
	}

	obj, err := repo.Get(context.Background(), ticket.UploadID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if obj.Confirmed {
		t.Error("RequestUpload() created an already-confirmed object")
	}
	if obj.ChatID != "chat-1" || obj.UploaderID != "user-a" {
		t.Errorf("RequestUpload() object = %+v, want ChatID=chat-1 UploaderID=user-a", obj)
	}
}

func TestMediaService_RequestUpload_NotMember(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.RequestUpload(context.Background(), "chat-1", "user-b", "image/png", 1024)
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("RequestUpload() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestMediaService_RequestUpload_EmptyContentType(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "", 1024)
	if !errors.Is(err, domain.ErrEmptyContentType) {
		t.Errorf("RequestUpload() error = %v, want %v", err, domain.ErrEmptyContentType)
	}
}

func TestMediaService_RequestUpload_InvalidSize(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 0)
	if !errors.Is(err, domain.ErrInvalidSize) {
		t.Errorf("RequestUpload() error = %v, want %v", err, domain.ErrInvalidSize)
	}
}

func TestMediaService_RequestUpload_SizeTooLarge(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", MaxUploadSizeBytes+1)
	if !errors.Is(err, domain.ErrSizeTooLarge) {
		t.Errorf("RequestUpload() error = %v, want %v", err, domain.ErrSizeTooLarge)
	}
}

func TestMediaService_RequestUpload_BlockedContentType(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	blocked := []string{
		"application/x-msdownload",
		"application/x-msdos-program",
		"application/x-executable",
		"application/x-mach-binary",
		"application/x-sh",
		"application/x-bat",
		"application/x-msi",
		"application/vnd.microsoft.portable-executable",
		"application/x-apple-diskimage",
		"application/java-archive",
	}

	for _, ct := range blocked {
		t.Run(ct, func(t *testing.T) {
			_, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", ct, 1024)
			if !errors.Is(err, domain.ErrContentTypeBlocked) {
				t.Errorf("RequestUpload(%q) error = %v, want %v", ct, err, domain.ErrContentTypeBlocked)
			}
		})
	}
}

func TestMediaService_RequestUpload_ChecksMembershipBeforeBlockedType(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.RequestUpload(context.Background(), "chat-1", "user-outsider", "application/x-msdownload", 1024)
	if !errors.Is(err, domain.ErrContentTypeBlocked) {
		t.Errorf("RequestUpload() error = %v, want %v", err, domain.ErrContentTypeBlocked)
	}
}

func TestMediaService_ConfirmUpload_Success(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}

	obj, _ := repo.Get(context.Background(), ticket.UploadID)
	store.existing[obj.ObjectKey] = true

	mediaID, err := svc.ConfirmUpload(context.Background(), ticket.UploadID, "user-a")
	if err != nil {
		t.Fatalf("ConfirmUpload() unexpected error: %v", err)
	}
	if mediaID != ticket.UploadID {
		t.Errorf("ConfirmUpload() mediaID = %q, want %q", mediaID, ticket.UploadID)
	}

	confirmed, _ := repo.Get(context.Background(), ticket.UploadID)
	if !confirmed.Confirmed {
		t.Error("ConfirmUpload() did not mark the object as confirmed")
	}
}

func TestMediaService_ConfirmUpload_ObjectNotUploaded(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}

	_, err = svc.ConfirmUpload(context.Background(), ticket.UploadID, "user-a")
	if !errors.Is(err, domain.ErrUploadNotConfirmed) {
		t.Errorf("ConfirmUpload() error = %v, want %v", err, domain.ErrUploadNotConfirmed)
	}
}

func TestMediaService_ConfirmUpload_WrongUploader(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a", "user-b")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}

	_, err = svc.ConfirmUpload(context.Background(), ticket.UploadID, "user-b")
	if !errors.Is(err, domain.ErrMediaNotFound) {
		t.Errorf("ConfirmUpload() error = %v, want %v", err, domain.ErrMediaNotFound)
	}
}

func TestMediaService_ConfirmUpload_NotFound(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.ConfirmUpload(context.Background(), "missing-id", "user-a")
	if !errors.Is(err, domain.ErrMediaNotFound) {
		t.Errorf("ConfirmUpload() error = %v, want %v", err, domain.ErrMediaNotFound)
	}
}

func TestMediaService_GetDownloadURL_Success(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a", "user-b")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}
	obj, _ := repo.Get(context.Background(), ticket.UploadID)
	store.existing[obj.ObjectKey] = true
	if _, err := svc.ConfirmUpload(context.Background(), ticket.UploadID, "user-a"); err != nil {
		t.Fatalf("ConfirmUpload() unexpected error: %v", err)
	}

	dl, err := svc.GetDownloadURL(context.Background(), ticket.UploadID, "user-b")
	if err != nil {
		t.Fatalf("GetDownloadURL() unexpected error: %v", err)
	}
	if dl.DownloadURL == "" {
		t.Error("GetDownloadURL() returned empty DownloadURL")
	}
}

func TestMediaService_GetDownloadURL_NotConfirmed(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}

	_, err = svc.GetDownloadURL(context.Background(), ticket.UploadID, "user-a")
	if !errors.Is(err, domain.ErrUploadNotConfirmed) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrUploadNotConfirmed)
	}
}

func TestMediaService_GetDownloadURL_NotMember(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	ticket, err := svc.RequestUpload(context.Background(), "chat-1", "user-a", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload() unexpected error: %v", err)
	}
	obj, _ := repo.Get(context.Background(), ticket.UploadID)
	store.existing[obj.ObjectKey] = true
	if _, err := svc.ConfirmUpload(context.Background(), ticket.UploadID, "user-a"); err != nil {
		t.Fatalf("ConfirmUpload() unexpected error: %v", err)
	}

	_, err = svc.GetDownloadURL(context.Background(), ticket.UploadID, "user-outsider")
	if !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestMediaService_GetDownloadURL_NotFound(t *testing.T) {
	repo := newFakeMediaRepository()
	store := newFakeObjectStore()
	members := newFakeChatMembership("chat-1", "user-a")
	svc := NewMediaService(repo, store, members)

	_, err := svc.GetDownloadURL(context.Background(), "missing-id", "user-a")
	if !errors.Is(err, domain.ErrMediaNotFound) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrMediaNotFound)
	}
}
