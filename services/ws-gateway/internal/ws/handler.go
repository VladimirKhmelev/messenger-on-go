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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

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
	jwtSecret string
	chat      ChatClient
	registry  *Registry
	presence  PresencePublisher
}

func NewHandler(jwtSecret string, chat ChatClient, registry *Registry, presence PresencePublisher) *Handler {
	return &Handler{jwtSecret: jwtSecret, chat: chat, registry: registry, presence: presence}
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

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws-gateway: upgrade failed for user %s: %v", userID, err)
		return
	}

	session := newSession(userID, token, conn, h.chat, h.presence)
	session.tokenExpiresAt = expiresAt
	session.run(h.registry)
}
