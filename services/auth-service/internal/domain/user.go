package domain

import "time"

type User struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	Tag          string    `db:"tag"`
	PasswordHash string    `db:"password_hash"`
	CreatedAt    time.Time `db:"created_at"`
}
