package service

import (
	"net/http"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

const MaxGroupAvatarSizeBytes = 2 * 1024 * 1024 // 2MB

var validReportCategories = map[domain.ReportCategory]bool{
	domain.ReportCategorySpam:  true,
	domain.ReportCategoryAbuse: true,
	domain.ReportCategoryOther: true,
}

var allowedGroupAvatarContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

func validateMessageBody(body string) error {
	if body == "" {
		return domain.ErrEmptyMessage
	}
	if len(body) > MaxMessageBodyBytes {
		return domain.ErrMessageTooLarge
	}
	return nil
}

func validateGroupName(name string) error {
	if name == "" {
		return domain.ErrGroupNameRequired
	}
	return nil
}

func validateMemberRole(role domain.MemberRole) error {
	if role != domain.MemberRoleAdmin && role != domain.MemberRoleMember {
		return domain.ErrInvalidRole
	}
	return nil
}

func validateReportCategory(category domain.ReportCategory) error {
	if !validReportCategories[category] {
		return domain.ErrInvalidReportCategory
	}
	return nil
}

func validateGroupAvatar(data []byte) (string, error) {
	if len(data) > MaxGroupAvatarSizeBytes {
		return "", domain.ErrGroupAvatarTooLarge
	}
	contentType := http.DetectContentType(data)
	if !allowedGroupAvatarContentTypes[contentType] {
		return "", domain.ErrInvalidGroupAvatarType
	}
	return contentType, nil
}
