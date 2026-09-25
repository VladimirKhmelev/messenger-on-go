package events

import "time"

type MessageCreated struct {
	MessageID string    `json:"message_id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
}

type UserRegistered struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"created_at"`
}

type UserPasswordReset struct {
	UserID string    `json:"user_id"`
	Email  string    `json:"email"`
	At     time.Time `json:"at"`
}

type UserOAuthLinked struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Provider string    `json:"provider"`
	At       time.Time `json:"at"`
}

type NotifyPush struct {
	UserID    string    `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}
