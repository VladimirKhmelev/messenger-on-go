package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
)

type clientMessage struct {
	Type   string `json:"type"`
	ChatID string `json:"chat_id"`
	Text   string `json:"text,omitempty"`
	Limit  int32  `json:"limit,omitempty"`
}

type serverMessage struct {
	Type      string               `json:"type"`
	Error     string               `json:"error,omitempty"`
	MessageID string               `json:"message_id,omitempty"`
	Messages  []chatclient.Message `json:"messages,omitempty"`
	ChatID    string               `json:"chat_id,omitempty"`
	Message   *chatclient.Message  `json:"message,omitempty"`
}

type session struct {
	userID string
	token  string
	conn   *websocket.Conn
	chat   ChatClient

	writeMu sync.Mutex
}

func newSession(userID, token string, conn *websocket.Conn, chat ChatClient) *session {
	return &session{userID: userID, token: token, conn: conn, chat: chat}
}

func (s *session) run(registry *Registry) {
	registry.add(s)
	defer registry.remove(s)

	defer func() { _ = s.conn.Close() }()

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			log.Printf("ws-gateway: connection closed for user %s: %v", s.userID, err)
			return
		}

		s.handle(data)
	}
}

func (s *session) handle(data []byte) {
	var msg clientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.writeError("invalid message format")
		return
	}

	ctx := context.Background()

	switch msg.Type {
	case "send_message":
		messageID, err := s.chat.SendMessage(ctx, s.token, msg.ChatID, msg.Text)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{Type: "message_sent", MessageID: messageID})

	case "get_history":
		messages, err := s.chat.GetHistory(ctx, s.token, msg.ChatID, msg.Limit)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{Type: "history", Messages: messages})

	default:
		s.writeError("unknown message type: " + msg.Type)
	}
}

func (s *session) writeError(message string) {
	s.write(serverMessage{Type: "error", Error: message})
}

func (s *session) write(msg serverMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws-gateway: failed to marshal message for user %s: %v", s.userID, err)
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		log.Printf("ws-gateway: failed to write message for user %s: %v", s.userID, err)
	}
}
