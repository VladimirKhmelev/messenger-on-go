package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

type PostgresChatRepository struct {
	conn *sqlx.DB
}

func NewPostgresChatRepository(dsn string) (*PostgresChatRepository, error) {
	conn, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, err
	}
	return &PostgresChatRepository{conn: conn}, nil
}

func (r *PostgresChatRepository) CreateChat(ctx context.Context, chat *domain.Chat, chatKeyByUserID map[string]MemberChatKey) error {
	tx, err := r.conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO chats (id, created_at) VALUES ($1, $2)`, chat.ID, chat.CreatedAt); err != nil {
		return err
	}

	for memberID, key := range chatKeyByUserID {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, joined_at, encrypted_chat_key, wrapped_for_public_key)
			VALUES ($1, $2, $3, $4, $5)`,
			chat.ID, memberID, chat.CreatedAt, key.EncryptedChatKey, key.WrappedForPublicKey,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresChatRepository) GetChat(ctx context.Context, chatID string) (*domain.Chat, error) {
	var chat domain.Chat
	err := r.conn.GetContext(ctx, &chat, `SELECT id, created_at FROM chats WHERE id = $1`, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrChatNotFound
	}
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

func (r *PostgresChatRepository) FindPrivateChat(ctx context.Context, userA, userB string) (*domain.Chat, error) {
	var chat domain.Chat
	err := r.conn.GetContext(ctx, &chat, `
		SELECT c.id, c.created_at FROM chats c
		WHERE EXISTS (SELECT 1 FROM chat_members WHERE chat_id = c.id AND user_id = $1)
		  AND EXISTS (SELECT 1 FROM chat_members WHERE chat_id = c.id AND user_id = $2)
		  AND (SELECT COUNT(*) FROM chat_members WHERE chat_id = c.id) = 2`,
		userA, userB,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrChatNotFound
	}
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

func (r *PostgresChatRepository) IsMember(ctx context.Context, chatID, userID string) (bool, error) {
	var exists bool
	err := r.conn.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`,
		chatID, userID,
	)
	return exists, err
}

func (r *PostgresChatRepository) ListMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error) {
	var members []*domain.ChatMember
	err := r.conn.SelectContext(ctx, &members, `
		SELECT chat_id, user_id, joined_at, last_read_message_id, last_read_at, encrypted_chat_key, wrapped_for_public_key
		FROM chat_members WHERE chat_id = $1 ORDER BY joined_at`,
		chatID,
	)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (r *PostgresChatRepository) UpdateChatKey(ctx context.Context, chatID, userID, encryptedChatKey, wrappedForPublicKey string) error {
	result, err := r.conn.ExecContext(ctx, `
		UPDATE chat_members SET encrypted_chat_key = $1, wrapped_for_public_key = $2
		WHERE chat_id = $3 AND user_id = $4`,
		encryptedChatKey, wrappedForPublicKey, chatID, userID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotChatMember
	}
	return nil
}

func (r *PostgresChatRepository) GetChatKeyForUser(ctx context.Context, chatID, userID string) (string, error) {
	var encryptedChatKey string
	err := r.conn.GetContext(ctx, &encryptedChatKey, `
		SELECT encrypted_chat_key FROM chat_members WHERE chat_id = $1 AND user_id = $2`,
		chatID, userID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotChatMember
	}
	if err != nil {
		return "", err
	}
	return encryptedChatKey, nil
}

func (r *PostgresChatRepository) MarkRead(ctx context.Context, chatID, userID, messageID string, readAt time.Time) error {
	_, err := r.conn.ExecContext(ctx, `
		UPDATE chat_members SET last_read_message_id = $1, last_read_at = $2
		WHERE chat_id = $3 AND user_id = $4`,
		messageID, readAt, chatID, userID,
	)
	return err
}

func (r *PostgresChatRepository) ListChatsForUser(ctx context.Context, userID string) ([]*domain.Chat, error) {
	var chats []*domain.Chat
	err := r.conn.SelectContext(ctx, &chats, `
		SELECT c.id, c.created_at FROM chats c
		JOIN chat_members m ON m.chat_id = c.id
		LEFT JOIN LATERAL (
			SELECT created_at FROM messages WHERE chat_id = c.id ORDER BY created_at DESC LIMIT 1
		) last_message ON true
		WHERE m.user_id = $1
		ORDER BY COALESCE(last_message.created_at, c.created_at) DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	return chats, nil
}

func (r *PostgresChatRepository) CreateMessage(ctx context.Context, message *domain.Message) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, body, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		message.ID, message.ChatID, message.SenderID, message.Body, message.CreatedAt,
	)
	return err
}

const messageProjectionColumns = `
	m.id, m.chat_id, m.sender_id,
	COALESCE(latest_edit.new_body, m.body) AS body,
	m.created_at,
	latest_edit.created_at AS edited_at,
	latest_delete.created_at AS deleted_at`

const messageProjectionLateralJoins = `
	LEFT JOIN LATERAL (
		SELECT new_body, created_at FROM message_events
		WHERE message_id = m.id AND event_type = 'edited'
		ORDER BY created_at DESC LIMIT 1
	) latest_edit ON true
	LEFT JOIN LATERAL (
		SELECT created_at FROM message_events
		WHERE message_id = m.id AND event_type = 'deleted_for_all'
		ORDER BY created_at DESC LIMIT 1
	) latest_delete ON true`

const messageProjectionJoins = `FROM messages m` + messageProjectionLateralJoins

func (r *PostgresChatRepository) ListMessages(ctx context.Context, chatID, requesterID string, limit, offset int) ([]*domain.Message, error) {
	var messages []*domain.Message
	err := r.conn.SelectContext(ctx, &messages, `
		SELECT `+messageProjectionColumns+` FROM (
			SELECT m.id `+messageProjectionJoins+`
			WHERE m.chat_id = $1
			  AND NOT EXISTS (
				SELECT 1 FROM message_hidden_for_user h WHERE h.message_id = m.id AND h.user_id = $3
			  )
			ORDER BY m.created_at DESC LIMIT $2 OFFSET $4
		) recent
		JOIN messages m ON m.id = recent.id
		`+messageProjectionLateralJoins+`
		ORDER BY m.created_at ASC`,
		chatID, limit, requesterID, offset,
	)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *PostgresChatRepository) GetLastMessage(ctx context.Context, chatID, requesterID string) (*domain.Message, error) {
	var message domain.Message
	err := r.conn.GetContext(ctx, &message, `
		SELECT `+messageProjectionColumns+`
		`+messageProjectionJoins+`
		WHERE m.chat_id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM message_hidden_for_user h WHERE h.message_id = m.id AND h.user_id = $2
		  )
		ORDER BY m.created_at DESC LIMIT 1`,
		chatID, requesterID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *PostgresChatRepository) GetMessage(ctx context.Context, messageID string) (*domain.Message, error) {
	var message domain.Message
	err := r.conn.GetContext(ctx, &message, `
		SELECT `+messageProjectionColumns+`
		`+messageProjectionJoins+`
		WHERE m.id = $1`,
		messageID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrMessageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *PostgresChatRepository) AppendMessageEvent(ctx context.Context, event *domain.MessageEvent) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO message_events (id, message_id, chat_id, actor_id, event_type, new_body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID, event.MessageID, event.ChatID, event.ActorID, event.Type, event.NewBody, event.CreatedAt,
	)
	return err
}

func (r *PostgresChatRepository) HideMessageForUser(ctx context.Context, messageID, userID string) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO message_hidden_for_user (message_id, user_id, hidden_at)
		VALUES ($1, $2, now())
		ON CONFLICT (message_id, user_id) DO NOTHING`,
		messageID, userID,
	)
	return err
}
