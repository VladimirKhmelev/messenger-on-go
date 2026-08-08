package repository

import (
	"context"
	"database/sql"
	"errors"

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

func (r *PostgresChatRepository) CreateChat(ctx context.Context, chat *domain.Chat, memberIDs []string) error {
	tx, err := r.conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO chats (id, created_at) VALUES ($1, $2)`, chat.ID, chat.CreatedAt); err != nil {
		return err
	}

	for _, memberID := range memberIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, joined_at) VALUES ($1, $2, $3)`,
			chat.ID, memberID, chat.CreatedAt,
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
		SELECT chat_id, user_id, joined_at FROM chat_members WHERE chat_id = $1 ORDER BY joined_at`,
		chatID,
	)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (r *PostgresChatRepository) ListChatsForUser(ctx context.Context, userID string) ([]*domain.Chat, error) {
	var chats []*domain.Chat
	err := r.conn.SelectContext(ctx, &chats, `
		SELECT c.id, c.created_at FROM chats c
		JOIN chat_members m ON m.chat_id = c.id
		WHERE m.user_id = $1
		ORDER BY c.created_at DESC`,
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

func (r *PostgresChatRepository) ListMessages(ctx context.Context, chatID string, limit int) ([]*domain.Message, error) {
	var messages []*domain.Message
	err := r.conn.SelectContext(ctx, &messages, `
		SELECT id, chat_id, sender_id, body, created_at FROM (
			SELECT id, chat_id, sender_id, body, created_at FROM messages
			WHERE chat_id = $1 ORDER BY created_at DESC LIMIT $2
		) recent ORDER BY created_at ASC`,
		chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *PostgresChatRepository) GetLastMessage(ctx context.Context, chatID string) (*domain.Message, error) {
	var message domain.Message
	err := r.conn.GetContext(ctx, &message, `
		SELECT id, chat_id, sender_id, body, created_at FROM messages
		WHERE chat_id = $1 ORDER BY created_at DESC LIMIT 1`,
		chatID,
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
		SELECT id, chat_id, sender_id, body, created_at FROM messages WHERE id = $1`,
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
