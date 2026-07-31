package ws

import (
	"log"

	"github.com/gorilla/websocket"
)

type session struct {
	userID string
	conn   *websocket.Conn
}

func newSession(userID string, conn *websocket.Conn) *session {
	return &session{userID: userID, conn: conn}
}

func (s *session) run() {
	defer func() { _ = s.conn.Close() }()

	for {
		_, _, err := s.conn.ReadMessage()
		if err != nil {
			log.Printf("ws-gateway: connection closed for user %s: %v", s.userID, err)
			return
		}
	}
}
