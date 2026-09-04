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

const (
	maxOpenConns    = 20
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
)

type PostgresChatRepository struct {
	conn *sqlx.DB
}

func NewPostgresChatRepository(dsn string) (*PostgresChatRepository, error) {
	conn, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(maxOpenConns)
	conn.SetMaxIdleConns(maxIdleConns)
	conn.SetConnMaxLifetime(connMaxLifetime)
	return &PostgresChatRepository{conn: conn}, nil
}

func (r *PostgresChatRepository) CreateChat(ctx context.Context, chat *domain.Chat, chatKeyByUserID map[string]MemberChatKey) error {
	tx, err := r.conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chats (id, created_at, chat_type, name, created_by) VALUES ($1, $2, $3, $4, $5)`,
		chat.ID, chat.CreatedAt, chat.ChatType, chat.Name, chat.CreatedBy,
	); err != nil {
		return err
	}

	for memberID, key := range chatKeyByUserID {
		role := domain.MemberRoleMember
		if chat.CreatedBy != nil && memberID == *chat.CreatedBy {
			role = domain.MemberRoleAdmin
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, joined_at, encrypted_chat_key, wrapped_for_public_key, role)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			chat.ID, memberID, chat.CreatedAt, key.EncryptedChatKey, key.WrappedForPublicKey, role,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresChatRepository) GetChat(ctx context.Context, chatID string) (*domain.Chat, error) {
	var chat domain.Chat
	err := r.conn.GetContext(ctx, &chat, `
		SELECT id, created_at, chat_type, name, created_by FROM chats WHERE id = $1`, chatID)
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
		SELECT c.id, c.created_at, c.chat_type, c.name, c.created_by FROM chats c
		WHERE c.chat_type = 'private'
		  AND EXISTS (SELECT 1 FROM chat_members WHERE chat_id = c.id AND user_id = $1)
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

func (r *PostgresChatRepository) IsAdmin(ctx context.Context, chatID, userID string) (bool, error) {
	var exists bool
	err := r.conn.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2 AND role = 'admin')`,
		chatID, userID,
	)
	return exists, err
}

func (r *PostgresChatRepository) GetMember(ctx context.Context, chatID, userID string) (*domain.ChatMember, error) {
	var member domain.ChatMember
	err := r.conn.GetContext(ctx, &member, `
		SELECT chat_id, user_id, joined_at, last_read_message_id, last_read_at,
		       encrypted_chat_key, wrapped_for_public_key, role
		FROM chat_members WHERE chat_id = $1 AND user_id = $2`,
		chatID, userID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotChatMember
	}
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *PostgresChatRepository) MemberCount(ctx context.Context, chatID string) (int, error) {
	var count int
	err := r.conn.GetContext(ctx, &count, `SELECT COUNT(*) FROM chat_members WHERE chat_id = $1`, chatID)
	return count, err
}

func (r *PostgresChatRepository) AddMember(ctx context.Context, chatID, userID string, key MemberChatKey) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id, joined_at, encrypted_chat_key, wrapped_for_public_key, role)
		VALUES ($1, $2, now(), $3, $4, 'member')`,
		chatID, userID, key.EncryptedChatKey, key.WrappedForPublicKey,
	)
	return err
}

func (r *PostgresChatRepository) RemoveMember(ctx context.Context, chatID, userID string) error {
	result, err := r.conn.ExecContext(ctx, `DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
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

func (r *PostgresChatRepository) SetRole(ctx context.Context, chatID, userID string, role domain.MemberRole) error {
	result, err := r.conn.ExecContext(ctx, `
		UPDATE chat_members SET role = $1 WHERE chat_id = $2 AND user_id = $3`,
		role, chatID, userID,
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

func (r *PostgresChatRepository) ListMembers(ctx context.Context, chatID string) ([]*domain.ChatMember, error) {
	var members []*domain.ChatMember
	err := r.conn.SelectContext(ctx, &members, `
		SELECT chat_id, user_id, joined_at, last_read_message_id, last_read_at,
		       encrypted_chat_key, wrapped_for_public_key, role
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
		SELECT c.id, c.created_at, c.chat_type, c.name, c.created_by FROM chats c
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
