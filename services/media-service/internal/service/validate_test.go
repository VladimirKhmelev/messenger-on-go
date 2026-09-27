package service

import (
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		size        int64
		want        error
	}{
		{"ok", "image/png", 1024, nil},
		{"exactly max size", "image/png", MaxUploadSizeBytes, nil},
		{"empty content type", "", 1024, domain.ErrEmptyContentType},
		{"zero size", "image/png", 0, domain.ErrInvalidSize},
		{"negative size", "image/png", -1, domain.ErrInvalidSize},
		{"one byte over max", "image/png", MaxUploadSizeBytes + 1, domain.ErrSizeTooLarge},
		{"blocked type wins over bad size", "application/x-msdownload", 0, domain.ErrContentTypeBlocked},
	}
	for _, ct := range []string{
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
		"application/vnd.android.package-archive",
		"application/x-itunes-ipa",
	} {
		tests = append(tests, struct {
			name        string
			contentType string
			size        int64
			want        error
		}{"blocked " + ct, ct, 1024, domain.ErrContentTypeBlocked})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateUpload(tt.contentType, tt.size); !errors.Is(got, tt.want) {
				t.Fatalf("validateUpload(%q, %d) = %v, want %v", tt.contentType, tt.size, got, tt.want)
			}
		})
	}
}
