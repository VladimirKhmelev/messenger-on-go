package domain

import "time"

type MessageCreated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageUpdated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	NewBody   *string   `json:"new_body,omitempty"`
	Deleted   bool      `json:"deleted"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MessageRead struct {
	ChatID    string    `json:"chat_id"`
	UserID    string    `json:"user_id"`
	MessageID string    `json:"message_id"`
	ReadAt    time.Time `json:"read_at"`
}

type ChatDeleted struct {
	ChatID        string    `json:"chat_id"`
	MemberUserIDs []string  `json:"member_user_ids"`
	DeletedAt     time.Time `json:"deleted_at"`
}

type NotifyPush struct {
	UserID    string    `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}

type PresenceChanged struct {
	UserID       string `json:"user_id"`
	Online       bool   `json:"online"`
	LastSeenUnix int64  `json:"last_seen_unix"`
}

type ProfileUpdated struct {
	UserID      string `json:"user_id"`
	Tag         string `json:"tag"`
	DisplayName string `json:"display_name"`
}

type TypingChanged struct {
	ChatID string `json:"chat_id"`
	UserID string `json:"user_id"`
}
