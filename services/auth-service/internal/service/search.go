package service

import (
	"context"
	"unicode/utf8"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	SearchUsersMinQueryLen = 3
	SearchUsersLimit       = 15
)

func (s *AuthService) GetUserByTag(ctx context.Context, tag string) (*domain.User, error) {
	return s.users.GetByTag(ctx, tag)
}

func (s *AuthService) SearchUsers(ctx context.Context, query string) ([]*domain.User, error) {
	if utf8.RuneCountInString(query) < SearchUsersMinQueryLen {
		return nil, domain.ErrSearchQueryTooShort
	}

	return s.users.SearchByTagPrefix(ctx, query, SearchUsersLimit)
}
