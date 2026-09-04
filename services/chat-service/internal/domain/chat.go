package domain

import "time"

type ChatType string

const (
	ChatTypePrivate ChatType = "private"
	ChatTypeGroup   ChatType = "group"
)

const MaxGroupChatMembers = 50

type Chat struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	ChatType  ChatType  `db:"chat_type"`
	Name      *string   `db:"name"`
	CreatedBy *string   `db:"created_by"`
}

type MemberRole string

const (
	MemberRoleAdmin  MemberRole = "admin"
	MemberRoleMember MemberRole = "member"
)

type ChatMember struct {
	ChatID              string     `db:"chat_id"`
	UserID              string     `db:"user_id"`
	JoinedAt            time.Time  `db:"joined_at"`
	LastReadMessageID   *string    `db:"last_read_message_id"`
	LastReadAt          *time.Time `db:"last_read_at"`
	EncryptedChatKey    string     `db:"encrypted_chat_key"`
	WrappedForPublicKey string     `db:"wrapped_for_public_key"`
	Role                MemberRole `db:"role"`
}

type Message struct {
	ID        string    `db:"id"`
	ChatID    string    `db:"chat_id"`
	SenderID  string    `db:"sender_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`

	EditedAt  *time.Time `db:"edited_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

type MessageEventType string

const (
	MessageEventEdited        MessageEventType = "edited"
	MessageEventDeletedForAll MessageEventType = "deleted_for_all"
)

type MessageEvent struct {
	ID        string           `db:"id"`
	MessageID string           `db:"message_id"`
	ChatID    string           `db:"chat_id"`
	ActorID   string           `db:"actor_id"`
	Type      MessageEventType `db:"event_type"`
	NewBody   *string          `db:"new_body"`
	CreatedAt time.Time        `db:"created_at"`
}
