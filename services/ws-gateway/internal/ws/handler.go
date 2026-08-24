package ws

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

type ChatClient interface {
	SendMessage(ctx context.Context, bearerToken, chatID, text string) (string, error)
	GetHistory(ctx context.Context, bearerToken, chatID string, limit, offset int32) ([]chatclient.Message, error)
	GetPresence(ctx context.Context, userID string) (online bool, lastSeenUnix int64, err error)
	SetOffline(ctx context.Context, userID string) error
	EditMessage(ctx context.Context, bearerToken, chatID, messageID, text string) error
	DeleteMessageForAll(ctx context.Context, bearerToken, chatID, messageID string) error
	DeleteMessageForMe(ctx context.Context, bearerToken, chatID, messageID string) error
	MarkRead(ctx context.Context, bearerToken, chatID, messageID string) error
	GetReadStatus(ctx context.Context, chatID, userID string) (string, error)
	SetTyping(ctx context.Context, chatID, userID string) error
}

type PresencePublisher interface {
	PublishPresenceChanged(event events.PresenceChanged) error
	PublishTypingChanged(event events.TypingChanged) error
}

type Handler struct {
	jwtSecret      string
	chat           ChatClient
	registry       *Registry
	presence       PresencePublisher
	allowedOrigins map[string]bool
	upgrader       websocket.Upgrader
}

func NewHandler(jwtSecret string, chat ChatClient, registry *Registry, presence PresencePublisher, allowedOrigins []string) *Handler {
	origins := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o != "" {
			origins[o] = true
		}
	}

	h := &Handler{jwtSecret: jwtSecret, chat: chat, registry: registry, presence: presence, allowedOrigins: origins}
	h.upgrader = websocket.Upgrader{CheckOrigin: h.checkOrigin}
	return h
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	if len(h.allowedOrigins) == 0 {
		return true
	}
	return h.allowedOrigins[r.Header.Get("Origin")]
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token query parameter", http.StatusUnauthorized)
		return
	}

	userID, expiresAt, err := jwtutil.ValidateAccessTokenWithExpiry(token, h.jwtSecret)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws-gateway: upgrade failed for user %s: %v", userID, err)
		return
	}

	session := newSession(userID, token, conn, h.chat, h.presence)
	session.tokenExpiresAt = expiresAt
	session.run(h.registry)
}
