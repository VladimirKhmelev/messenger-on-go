package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
)

const (
	maxOpenConns    = 20
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
)

type PostgresMediaRepository struct {
	conn *sqlx.DB
}

func NewPostgresMediaRepository(dsn string) (*PostgresMediaRepository, error) {
	conn, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(maxOpenConns)
	conn.SetMaxIdleConns(maxIdleConns)
	conn.SetConnMaxLifetime(connMaxLifetime)
	return &PostgresMediaRepository{conn: conn}, nil
}

func (r *PostgresMediaRepository) CreatePending(ctx context.Context, obj *domain.MediaObject) error {
	_, err := r.conn.ExecContext(ctx, `
		INSERT INTO media_objects (id, chat_id, uploader_id, object_key, content_type, size_bytes, confirmed, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, false, $7)`,
		obj.ID, obj.ChatID, obj.UploaderID, obj.ObjectKey, obj.ContentType, obj.SizeBytes, obj.CreatedAt,
	)
	return err
}

func (r *PostgresMediaRepository) Confirm(ctx context.Context, id string) error {
	result, err := r.conn.ExecContext(ctx, `UPDATE media_objects SET confirmed = true WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrMediaNotFound
	}
	return nil
}

func (r *PostgresMediaRepository) Get(ctx context.Context, id string) (*domain.MediaObject, error) {
	var obj domain.MediaObject
	err := r.conn.GetContext(ctx, &obj, `
		SELECT id, chat_id, uploader_id, object_key, content_type, size_bytes, confirmed, created_at
		FROM media_objects WHERE id = $1`,
		id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrMediaNotFound
	}
	if err != nil {
		return nil, err
	}
	return &obj, nil
}
