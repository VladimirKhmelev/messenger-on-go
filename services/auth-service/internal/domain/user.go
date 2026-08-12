package domain

import "time"

type User struct {
	ID            string    `db:"id"`
	Email         string    `db:"email"`
	Tag           string    `db:"tag"`
	DisplayName   string    `db:"display_name"`
	PasswordHash  string    `db:"password_hash"`
	EmailVerified bool      `db:"email_verified"`
	CreatedAt     time.Time `db:"created_at"`
}
