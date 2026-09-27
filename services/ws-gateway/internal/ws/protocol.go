package ws

import (
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/hub"
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
	Type              string        `json:"type"`
	Error             string        `json:"error,omitempty"`
	MessageID         string        `json:"message_id,omitempty"`
	Messages          []wireMessage `json:"messages,omitempty"`
	ChatID            string        `json:"chat_id,omitempty"`
	Message           *wireMessage  `json:"message,omitempty"`
	PeerUserID        string        `json:"peer_user_id,omitempty"`
	PeerTag           string        `json:"peer_tag,omitempty"`
	PeerDisplayName   string        `json:"peer_display_name,omitempty"`
	Online            bool          `json:"online,omitempty"`
	LastSeenUnix      int64         `json:"last_seen_unix,omitempty"`
	NewText           *string       `json:"new_text,omitempty"`
	Deleted           bool          `json:"deleted,omitempty"`
	Offset            int32         `json:"offset,omitempty"`
	LastReadMessageID string        `json:"last_read_message_id,omitempty"`
}

type wireMessage struct {
	MessageID     string `json:"message_id"`
	SenderUserID  string `json:"sender_user_id"`
	Text          string `json:"text"`
	CreatedAtUnix int64  `json:"created_at_unix"`
	EditedAtUnix  int64  `json:"edited_at_unix"`
	Deleted       bool   `json:"deleted"`
}

func toWireMessage(m domain.Message) wireMessage {
	return wireMessage{
		MessageID:     m.MessageID,
		SenderUserID:  m.SenderUserID,
		Text:          m.Text,
		CreatedAtUnix: m.CreatedAtUnix,
		EditedAtUnix:  m.EditedAtUnix,
		Deleted:       m.Deleted,
	}
}

func toWireMessages(ms []domain.Message) []wireMessage {
	out := make([]wireMessage, len(ms))
	for i, m := range ms {
		out[i] = toWireMessage(m)
	}
	return out
}

func toServerMessage(notification any) (serverMessage, bool) {
	switch n := notification.(type) {
	case hub.MessageReceived:
		wire := toWireMessage(n.Message)
		return serverMessage{Type: "message_received", ChatID: n.ChatID, Message: &wire}, true
	case domain.MessageUpdated:
		return serverMessage{Type: "message_updated", ChatID: n.ChatID, MessageID: n.MessageID, NewText: n.NewBody, Deleted: n.Deleted}, true
	case domain.MessageRead:
		return serverMessage{Type: "read_status", ChatID: n.ChatID, PeerUserID: n.UserID, LastReadMessageID: n.MessageID}, true
	case domain.NotifyPush:
		return serverMessage{Type: "notify_push", ChatID: n.ChatID, MessageID: n.MessageID}, true
	case domain.PresenceChanged:
		return serverMessage{Type: "presence_changed", PeerUserID: n.UserID, Online: n.Online, LastSeenUnix: n.LastSeenUnix}, true
	case domain.TypingChanged:
		return serverMessage{Type: "typing_changed", ChatID: n.ChatID, PeerUserID: n.UserID}, true
	case domain.ChatDeleted:
		return serverMessage{Type: "chat_deleted", ChatID: n.ChatID}, true
	case domain.ProfileUpdated:
		return serverMessage{Type: "profile_updated", PeerUserID: n.UserID, PeerTag: n.Tag, PeerDisplayName: n.DisplayName}, true
	default:
		return serverMessage{}, false
	}
}
