package domain

import "time"

type MediaObject struct {
	ID          string    `db:"id"`
	ChatID      string    `db:"chat_id"`
	UploaderID  string    `db:"uploader_id"`
	ObjectKey   string    `db:"object_key"`
	ContentType string    `db:"content_type"`
	SizeBytes   int64     `db:"size_bytes"`
	Confirmed   bool      `db:"confirmed"`
	CreatedAt   time.Time `db:"created_at"`
}
