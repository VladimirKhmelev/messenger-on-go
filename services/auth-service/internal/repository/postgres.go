package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

type PostgresUserRepository struct {
	conn *sqlx.DB
}

func NewPostgresUserRepository(dsn string) (*PostgresUserRepository, error) {
	conn, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, err
	}
	return &PostgresUserRepository{conn: conn}, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO users (id, email, tag, display_name, password_hash, email_verified, created_at, public_key, wrapped_private_key, key_wrap_salt)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		user.ID, user.Email, user.Tag, user.DisplayName, user.PasswordHash, user.EmailVerified, user.CreatedAt,
		user.PublicKey, user.WrappedPrivateKey, user.KeyWrapSalt,
	)
	return err
}

func (r *PostgresUserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.conn.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email)
	return exists, err
}

func (r *PostgresUserRepository) ExistsByTag(ctx context.Context, tag string) (bool, error) {
	var exists bool
	err := r.conn.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE tag = $1)`, tag)
	return exists, err
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.conn.GetContext(ctx, &user, `SELECT id, email, tag, display_name, password_hash, email_verified, created_at, public_key, wrapped_private_key, key_wrap_salt FROM users WHERE email = $1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *PostgresUserRepository) GetByTag(ctx context.Context, tag string) (*domain.User, error) {
	var user domain.User
	err := r.conn.GetContext(ctx, &user, `SELECT id, email, tag, display_name, password_hash, email_verified, created_at, public_key, wrapped_private_key, key_wrap_salt FROM users WHERE tag = $1`, tag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.conn.GetContext(ctx, &user, `SELECT id, email, tag, display_name, password_hash, email_verified, created_at, public_key, wrapped_private_key, key_wrap_salt FROM users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *PostgresUserRepository) SearchByTagPrefix(ctx context.Context, prefix string, limit int) ([]*domain.User, error) {
	pattern := escapeLikePattern(prefix) + "%"

	var users []*domain.User
	err := r.conn.SelectContext(ctx, &users, `
		SELECT id, email, tag, display_name, password_hash, email_verified, created_at, public_key, wrapped_private_key, key_wrap_salt FROM users
		WHERE tag LIKE $1 ESCAPE '\' ORDER BY tag LIMIT $2`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (r *PostgresUserRepository) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := r.conn.ExecContext(ctx, `UPDATE users SET email_verified = TRUE WHERE id = $1`, userID)
	return err
}

func (r *PostgresUserRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	_, err := r.conn.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID)
	return err
}

func (r *PostgresUserRepository) UpdateTag(ctx context.Context, userID, tag string) error {
	_, err := r.conn.ExecContext(ctx, `UPDATE users SET tag = $1 WHERE id = $2`, tag, userID)
	return err
}

func (r *PostgresUserRepository) UpdateDisplayName(ctx context.Context, userID, displayName string) error {
	_, err := r.conn.ExecContext(ctx, `UPDATE users SET display_name = $1 WHERE id = $2`, displayName, userID)
	return err
}

func (r *PostgresUserRepository) UpdatePublicKey(ctx context.Context, userID, publicKey string) error {
	_, err := r.conn.ExecContext(ctx, `UPDATE users SET public_key = $1 WHERE id = $2`, publicKey, userID)
	return err
}

func (r *PostgresUserRepository) UpdateWrappedPrivateKey(ctx context.Context, userID, wrappedPrivateKey, keyWrapSalt string) error {
	_, err := r.conn.ExecContext(ctx, `
		UPDATE users SET wrapped_private_key = $1, key_wrap_salt = $2 WHERE id = $3`,
		wrappedPrivateKey, keyWrapSalt, userID,
	)
	return err
}

func (r *PostgresUserRepository) UpsertAvatar(ctx context.Context, avatar *domain.Avatar) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO user_avatars (user_id, data, content_type, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET data = $2, content_type = $3, updated_at = $4`,
		avatar.UserID, avatar.Data, avatar.ContentType, avatar.UpdatedAt,
	)
	return err
}

func (r *PostgresUserRepository) GetAvatar(ctx context.Context, userID string) (*domain.Avatar, error) {
	var avatar domain.Avatar
	err := r.conn.GetContext(ctx, &avatar, `
		SELECT user_id, data, content_type, updated_at FROM user_avatars WHERE user_id = $1`,
		userID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrAvatarNotFound
	}
	if err != nil {
		return nil, err
	}
	return &avatar, nil
}

func (r *PostgresUserRepository) DeleteAvatar(ctx context.Context, userID string) error {
	_, err := r.conn.ExecContext(ctx, `DELETE FROM user_avatars WHERE user_id = $1`, userID)
	return err
}

func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
