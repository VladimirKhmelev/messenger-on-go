package avatar

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

const maxUploadBytes = 2*1024*1024 + 1024 

type AvatarService interface {
	UploadAvatar(ctx context.Context, userID string, data []byte) error
	GetAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
	IsAccessTokenStale(ctx context.Context, userID string, issuedAt time.Time) (bool, error)
}

type Handler struct {
	auth   AvatarService
	issuer *jwtutil.Issuer
}

func NewHandler(auth AvatarService, issuer *jwtutil.Issuer) *Handler {
	return &Handler{auth: auth, issuer: issuer}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
	userID := pathParams["user_id"]
	if userID == "" {
		http.Error(w, "missing user id", http.StatusBadRequest)
		return
	}

	avatar, err := h.auth.GetAvatar(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrAvatarNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", avatar.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Write(avatar.Data)
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "missing or invalid authorization", http.StatusUnauthorized)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxUploadBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := h.auth.UploadAvatar(r.Context(), userID, data); err != nil {
		writeDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authenticate(r *http.Request) (userID string, ok bool) {
	authHeader := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(authHeader, prefix)

	claims, err := h.issuer.Parse(token, jwtutil.TokenTypeAccess)
	if err != nil {
		return "", false
	}

	if claims.IssuedAt != nil {
		stale, err := h.auth.IsAccessTokenStale(r.Context(), claims.UserID, claims.IssuedAt.Time)
		if err != nil || stale {
			return "", false
		}
	}

	return claims.UserID, true
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAvatarTooLarge), errors.Is(err, domain.ErrInvalidAvatarType):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
