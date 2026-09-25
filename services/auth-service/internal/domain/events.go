package domain

import "time"

type UserRegistered struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"created_at"`
}

type UserPasswordReset struct {
	UserID string    `json:"user_id"`
	Email  string    `json:"email"`
	At     time.Time `json:"at"`
}

type UserOAuthLinked struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Provider string    `json:"provider"`
	At       time.Time `json:"at"`
}

type UserProfileUpdated struct {
	UserID      string `json:"user_id"`
	Tag         string `json:"tag"`
	DisplayName string `json:"display_name"`
}
