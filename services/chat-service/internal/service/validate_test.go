package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

func TestValidateMessageBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{"ok", "hello", nil},
		{"empty", "", domain.ErrEmptyMessage},
		{"exactly max size", strings.Repeat("a", MaxMessageBodyBytes), nil},
		{"one byte over max", strings.Repeat("a", MaxMessageBodyBytes+1), domain.ErrMessageTooLarge},
		{"multibyte over max", strings.Repeat("я", MaxMessageBodyBytes/2+1), domain.ErrMessageTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateMessageBody(tt.body); !errors.Is(got, tt.want) {
				t.Fatalf("validateMessageBody() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  error
	}{
		{"ok", "friends", nil},
		{"empty", "", domain.ErrGroupNameRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateGroupName(tt.input); !errors.Is(got, tt.want) {
				t.Fatalf("validateGroupName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateMemberRole(t *testing.T) {
	tests := []struct {
		name string
		role domain.MemberRole
		want error
	}{
		{"admin", domain.MemberRoleAdmin, nil},
		{"member", domain.MemberRoleMember, nil},
		{"empty", "", domain.ErrInvalidRole},
		{"unknown", "owner", domain.ErrInvalidRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateMemberRole(tt.role); !errors.Is(got, tt.want) {
				t.Fatalf("validateMemberRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateReportCategory(t *testing.T) {
	tests := []struct {
		name     string
		category domain.ReportCategory
		want     error
	}{
		{"spam", domain.ReportCategorySpam, nil},
		{"abuse", domain.ReportCategoryAbuse, nil},
		{"other", domain.ReportCategoryOther, nil},
		{"empty", "", domain.ErrInvalidReportCategory},
		{"unknown", "fraud", domain.ErrInvalidReportCategory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateReportCategory(tt.category); !errors.Is(got, tt.want) {
				t.Fatalf("validateReportCategory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateGroupAvatar(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantType string
		wantErr  error
	}{
		{"png", pngMagicBytes, "image/png", nil},
		{"plain text", []byte("hello"), "", domain.ErrInvalidGroupAvatarType},
		{"too large", append(pngMagicBytes, make([]byte, MaxGroupAvatarSizeBytes)...), "", domain.ErrGroupAvatarTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, err := validateGroupAvatar(tt.data)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateGroupAvatar() err = %v, want %v", err, tt.wantErr)
			}
			if gotType != tt.wantType {
				t.Fatalf("validateGroupAvatar() type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}
