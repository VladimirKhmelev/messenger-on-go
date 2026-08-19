package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

func testPNGBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() unexpected error: %v", err)
	}
	return buf.Bytes()
}

func testJPEGBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() unexpected error: %v", err)
	}
	return buf.Bytes()
}

func TestAuthService_UploadAvatar_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	pngData := testPNGBytes(t)
	if err := svc.UploadAvatar(context.Background(), user.ID, pngData); err != nil {
		t.Fatalf("UploadAvatar() unexpected error: %v", err)
	}

	avatar, err := svc.GetAvatar(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetAvatar() unexpected error: %v", err)
	}
	if avatar.ContentType != "image/png" {
		t.Errorf("GetAvatar() content type = %q, want %q", avatar.ContentType, "image/png")
	}
	if !bytes.Equal(avatar.Data, pngData) {
		t.Error("GetAvatar() data does not match uploaded bytes")
	}
}

func TestAuthService_UploadAvatar_AcceptsJPEG(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	jpegData := testJPEGBytes(t)
	if err := svc.UploadAvatar(context.Background(), user.ID, jpegData); err != nil {
		t.Fatalf("UploadAvatar() unexpected error: %v", err)
	}

	avatar, err := svc.GetAvatar(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetAvatar() unexpected error: %v", err)
	}
	if avatar.ContentType != "image/jpeg" {
		t.Errorf("GetAvatar() content type = %q, want %q", avatar.ContentType, "image/jpeg")
	}
}

func TestAuthService_UploadAvatar_RejectsNonImage(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err = svc.UploadAvatar(context.Background(), user.ID, []byte("not an image, just text pretending to be one"))
	if !errors.Is(err, domain.ErrInvalidAvatarType) {
		t.Errorf("UploadAvatar() error = %v, want %v", err, domain.ErrInvalidAvatarType)
	}
}

func TestAuthService_UploadAvatar_RejectsTooLarge(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	oversized := make([]byte, MaxAvatarSizeBytes+1)
	err = svc.UploadAvatar(context.Background(), user.ID, oversized)
	if !errors.Is(err, domain.ErrAvatarTooLarge) {
		t.Errorf("UploadAvatar() error = %v, want %v", err, domain.ErrAvatarTooLarge)
	}
}

func TestAuthService_GetAvatar_NotFound(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	_, err := svc.GetAvatar(context.Background(), "no-such-user")
	if !errors.Is(err, domain.ErrAvatarNotFound) {
		t.Errorf("GetAvatar() error = %v, want %v", err, domain.ErrAvatarNotFound)
	}
}

func TestAuthService_DeleteAvatar_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Name", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if err := svc.UploadAvatar(context.Background(), user.ID, testPNGBytes(t)); err != nil {
		t.Fatalf("UploadAvatar() unexpected error: %v", err)
	}

	if err := svc.DeleteAvatar(context.Background(), user.ID); err != nil {
		t.Fatalf("DeleteAvatar() unexpected error: %v", err)
	}

	_, err = svc.GetAvatar(context.Background(), user.ID)
	if !errors.Is(err, domain.ErrAvatarNotFound) {
		t.Errorf("GetAvatar() after delete error = %v, want %v", err, domain.ErrAvatarNotFound)
	}
}
