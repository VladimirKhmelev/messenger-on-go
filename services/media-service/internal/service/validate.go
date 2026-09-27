package service

import "github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"

var blockedContentTypes = map[string]bool{
	"application/x-msdownload":                      true, // .exe, .dll
	"application/x-msdos-program":                   true, // .exe, .com
	"application/x-executable":                      true, // ELF binaries (Linux)
	"application/x-mach-binary":                     true, // macOS executables
	"application/x-sh":                              true, // .sh
	"application/x-bat":                             true, // .bat
	"application/x-msi":                             true, // .msi
	"application/vnd.microsoft.portable-executable": true, // .exe, .dll (modern browsers)
	"application/x-apple-diskimage":                 true, // .dmg
	"application/java-archive":                      true, // .jar
	"application/vnd.android.package-archive":       true, // .apk
	"application/x-itunes-ipa":                      true, // .ipa
}

func validateUpload(contentType string, sizeBytes int64) error {
	if contentType == "" {
		return domain.ErrEmptyContentType
	}
	if blockedContentTypes[contentType] {
		return domain.ErrContentTypeBlocked
	}
	if sizeBytes <= 0 {
		return domain.ErrInvalidSize
	}
	if sizeBytes > MaxUploadSizeBytes {
		return domain.ErrSizeTooLarge
	}
	return nil
}
