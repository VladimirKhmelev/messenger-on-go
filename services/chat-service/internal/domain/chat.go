package domain

import "time"

type Chat struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

type ChatMember struct {
	ChatID   string    `db:"chat_id"`
	UserID   string    `db:"user_id"`
	JoinedAt time.Time `db:"joined_at"`
}

type Message struct {
	ID        string    `db:"id"`
	ChatID    string    `db:"chat_id"`
	SenderID  string    `db:"sender_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`
}
