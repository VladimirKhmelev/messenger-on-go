package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

const (
	pongWait      = 60 * time.Second
	pingPeriod    = (pongWait * 9) / 10
	maxReadBytes  = 256 * 1024
	handleTimeout = 10 * time.Second
)

type clientMessage struct {
	Type       string `json:"type"`
	ChatID     string `json:"chat_id"`
	MessageID  string `json:"message_id,omitempty"`
	Text       string `json:"text,omitempty"`
	Limit      int32  `json:"limit,omitempty"`
	Offset     int32  `json:"offset,omitempty"`
	PeerUserID string `json:"peer_user_id,omitempty"`
}

type serverMessage struct {
	Type              string               `json:"type"`
	Error             string               `json:"error,omitempty"`
	MessageID         string               `json:"message_id,omitempty"`
	Messages          []chatclient.Message `json:"messages,omitempty"`
	ChatID            string               `json:"chat_id,omitempty"`
	Message           *chatclient.Message  `json:"message,omitempty"`
	PeerUserID        string               `json:"peer_user_id,omitempty"`
	PeerTag           string               `json:"peer_tag,omitempty"`
	PeerDisplayName   string               `json:"peer_display_name,omitempty"`
	Online            bool                 `json:"online,omitempty"`
	LastSeenUnix      int64                `json:"last_seen_unix,omitempty"`
	NewText           *string              `json:"new_text,omitempty"`
	Deleted           bool                 `json:"deleted,omitempty"`
	Offset            int32                `json:"offset,omitempty"`
	LastReadMessageID string               `json:"last_read_message_id,omitempty"`
}

type session struct {
	userID         string
	token          string
	tokenExpiresAt time.Time
	conn           *websocket.Conn
	chat           ChatClient
	presence       PresencePublisher

	writeMu sync.Mutex
}

func newSession(userID, token string, conn *websocket.Conn, chat ChatClient, presence PresencePublisher) *session {
	return &session{userID: userID, token: token, conn: conn, chat: chat, presence: presence}
}

func (s *session) run(registry *Registry) {
	registry.add(s)
	defer func() {
		if hasOtherSessions := registry.remove(s); !hasOtherSessions {
			s.markOffline()
		}
	}()

	defer func() { _ = s.conn.Close() }()

	s.announcePresence(true, 0)

	s.conn.SetReadLimit(maxReadBytes)
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	stopPing := make(chan struct{})
	defer close(stopPing)
	go s.pingLoop(stopPing)

	stopExpiry := make(chan struct{})
	defer close(stopExpiry)
	go s.expiryWatcher(stopExpiry)

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			log.Printf("ws-gateway: connection closed for user %s: %v", s.userID, err)
			return
		}

		s.handle(data)
	}
}

func (s *session) expiryWatcher(stop <-chan struct{}) {
	if s.tokenExpiresAt.IsZero() {
		return
	}

	wait := time.Until(s.tokenExpiresAt)
	if wait < 0 {
		wait = 0
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-stop:
		return
	case <-timer.C:
		s.write(serverMessage{Type: "token_expired"})
		_ = s.conn.Close()
	}
}

func (s *session) pingLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.writeMu.Lock()
			err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			s.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (s *session) markOffline() {
	ctx, cancel := context.WithTimeout(context.Background(), handleTimeout)
	defer cancel()

	if err := s.chat.SetOffline(ctx, s.userID); err != nil {
		log.Printf("ws-gateway: failed to set user %s offline: %v", s.userID, err)
		return
	}

	_, lastSeenUnix, err := s.chat.GetPresence(ctx, s.userID)
	if err != nil {
		log.Printf("ws-gateway: failed to read last-seen for user %s: %v", s.userID, err)
		return
	}

	s.announcePresence(false, lastSeenUnix)
}

func (s *session) announcePresence(online bool, lastSeenUnix int64) {
	if err := s.presence.PublishPresenceChanged(events.PresenceChanged{
		UserID:       s.userID,
		Online:       online,
		LastSeenUnix: lastSeenUnix,
	}); err != nil {
		log.Printf("ws-gateway: failed to publish presence for user %s: %v", s.userID, err)
	}
}

func (s *session) handle(data []byte) {
	var msg clientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.writeError("invalid message format")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), handleTimeout)
	defer cancel()

	switch msg.Type {
	case "send_message":
		messageID, err := s.chat.SendMessage(ctx, s.token, msg.ChatID, msg.Text)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{Type: "message_sent", MessageID: messageID})

	case "get_history":
		messages, err := s.chat.GetHistory(ctx, s.token, msg.ChatID, msg.Limit, msg.Offset)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{Type: "history", ChatID: msg.ChatID, Messages: messages, Offset: msg.Offset})

	case "get_presence":
		online, lastSeenUnix, err := s.chat.GetPresence(ctx, msg.PeerUserID)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{
			Type:         "presence_changed",
			PeerUserID:   msg.PeerUserID,
			Online:       online,
			LastSeenUnix: lastSeenUnix,
		})

	case "edit_message":
		if err := s.chat.EditMessage(ctx, s.token, msg.ChatID, msg.MessageID, msg.Text); err != nil {
			s.writeError(err.Error())
			return
		}

	case "delete_message_for_all":
		if err := s.chat.DeleteMessageForAll(ctx, s.token, msg.ChatID, msg.MessageID); err != nil {
			s.writeError(err.Error())
			return
		}

	case "delete_message_for_me":
		if err := s.chat.DeleteMessageForMe(ctx, s.token, msg.ChatID, msg.MessageID); err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{Type: "message_updated", ChatID: msg.ChatID, MessageID: msg.MessageID, Deleted: true})

	case "mark_read":
		if err := s.chat.MarkRead(ctx, s.token, msg.ChatID, msg.MessageID); err != nil {
			s.writeError(err.Error())
			return
		}

	case "get_read_status":
		lastReadMessageID, err := s.chat.GetReadStatus(ctx, msg.ChatID, msg.PeerUserID)
		if err != nil {
			s.writeError(err.Error())
			return
		}
		s.write(serverMessage{
			Type:              "read_status",
			ChatID:            msg.ChatID,
			PeerUserID:        msg.PeerUserID,
			LastReadMessageID: lastReadMessageID,
		})

	case "start_typing":
		if err := s.chat.SetTyping(ctx, msg.ChatID, s.userID); err != nil {
			log.Printf("ws-gateway: failed to set typing for user %s in chat %s: %v", s.userID, msg.ChatID, err)
			return
		}
		if err := s.presence.PublishTypingChanged(events.TypingChanged{ChatID: msg.ChatID, UserID: s.userID}); err != nil {
			log.Printf("ws-gateway: failed to publish typing for user %s in chat %s: %v", s.userID, msg.ChatID, err)
		}

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
