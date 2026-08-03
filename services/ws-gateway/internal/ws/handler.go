package ws

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type ChatClient interface {
	SendMessage(ctx context.Context, bearerToken, chatID, text string) (string, error)
	GetHistory(ctx context.Context, bearerToken, chatID string, limit int32) ([]chatclient.Message, error)
}

type Handler struct {
	jwtSecret string
	chat      ChatClient
	registry  *Registry
}

func NewHandler(jwtSecret string, chat ChatClient, registry *Registry) *Handler {
	return &Handler{jwtSecret: jwtSecret, chat: chat, registry: registry}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token query parameter", http.StatusUnauthorized)
		return
	}

	userID, err := jwtutil.ValidateAccessToken(token, h.jwtSecret)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws-gateway: upgrade failed for user %s: %v", userID, err)
		return
	}

	session := newSession(userID, token, conn, h.chat)
	session.run(h.registry)
}
