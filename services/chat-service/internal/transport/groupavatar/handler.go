package groupavatar

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

const maxUploadBytes = 2*1024*1024 + 1024

type ChatService interface {
	UploadGroupAvatar(ctx context.Context, chatID, requesterID string, data []byte) error
	GetGroupAvatar(ctx context.Context, chatID string) (*domain.ChatAvatar, error)
}

type Handler struct {
	chat      ChatService
	jwtSecret string
}

func NewHandler(chat ChatService, jwtSecret string) *Handler {
	return &Handler{chat: chat, jwtSecret: jwtSecret}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
	chatID := pathParams["chat_id"]
	if chatID == "" {
		http.Error(w, "missing chat id", http.StatusBadRequest)
		return
	}

	avatar, err := h.chat.GetGroupAvatar(r.Context(), chatID)
	if err != nil {
		if errors.Is(err, domain.ErrGroupAvatarNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", avatar.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	if _, err := w.Write(avatar.Data); err != nil {
		log.Printf("chat-service: failed to write group avatar response for %s: %v", chatID, err)
	}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
	chatID := pathParams["chat_id"]
	if chatID == "" {
		http.Error(w, "missing chat id", http.StatusBadRequest)
		return
	}

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

	if err := h.chat.UploadGroupAvatar(r.Context(), chatID, userID, data); err != nil {
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

	userID, err := jwtutil.ValidateAccessToken(token, h.jwtSecret)
	if err != nil {
		return "", false
	}

	return userID, true
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrGroupAvatarTooLarge), errors.Is(err, domain.ErrInvalidGroupAvatarType):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, domain.ErrNotChatAdmin):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, domain.ErrNotGroupChat), errors.Is(err, domain.ErrChatNotFound):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
