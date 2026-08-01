package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Handler struct {
	jwtSecret string
}

func NewHandler(jwtSecret string) *Handler {
	return &Handler{jwtSecret: jwtSecret}
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

	session := newSession(userID, conn)
	session.run()
}
