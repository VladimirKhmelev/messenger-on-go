package repository

import (
	"context"
	"database/sql"
	"errors"

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
		INSERT INTO users (id, email, tag, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		user.ID, user.Email, user.Tag, user.PasswordHash, user.CreatedAt,
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
	err := r.conn.GetContext(ctx, &user, `SELECT id, email, tag, password_hash, created_at FROM users WHERE email = $1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
