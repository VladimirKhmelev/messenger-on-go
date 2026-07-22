package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func newTestAuthService(repo *fakeUserRepository) *AuthService {
	return NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist())
}

type fakeRateLimiter struct {
	allow bool
}

func newFakeRateLimiter() *fakeRateLimiter {
	return &fakeRateLimiter{allow: true}
}

func (l *fakeRateLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return l.allow, nil
}

type fakeTokenBlacklist struct {
	revoked map[string]bool
}

func newFakeTokenBlacklist() *fakeTokenBlacklist {
	return &fakeTokenBlacklist{revoked: map[string]bool{}}
}

func (b *fakeTokenBlacklist) Revoke(_ context.Context, token string, _ time.Duration) error {
	b.revoked[token] = true
	return nil
}

func (b *fakeTokenBlacklist) IsRevoked(_ context.Context, token string) (bool, error) {
	return b.revoked[token], nil
}

type fakeUserRepository struct {
	byEmail    map[string]bool
	byTag      map[string]bool
	users      map[string]*domain.User
	usersByTag map[string]*domain.User
	created    *domain.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		byEmail:    map[string]bool{},
		byTag:      map[string]bool{},
		users:      map[string]*domain.User{},
		usersByTag: map[string]*domain.User{},
	}
}

func (r *fakeUserRepository) Create(_ context.Context, user *domain.User) error {
	r.created = user
	r.users[user.Email] = user
	r.usersByTag[user.Tag] = user
	return nil
}

func (r *fakeUserRepository) ExistsByEmail(_ context.Context, email string) (bool, error) {
	return r.byEmail[email], nil
}

func (r *fakeUserRepository) ExistsByTag(_ context.Context, tag string) (bool, error) {
	return r.byTag[tag], nil
}

func (r *fakeUserRepository) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	user, ok := r.users[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

func (r *fakeUserRepository) GetByTag(_ context.Context, tag string) (*domain.User, error) {
	user, ok := r.usersByTag[tag]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

func (r *fakeUserRepository) SearchByTagPrefix(_ context.Context, prefix string, limit int) ([]*domain.User, error) {
	var matches []*domain.User
	for tag, user := range r.usersByTag {
		if strings.HasPrefix(tag, prefix) {
			matches = append(matches, user)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Tag < matches[j].Tag })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func TestAuthService_Register_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if user.Email != "user@example.com" || user.Tag != "balbes" {
		t.Errorf("Register() returned unexpected user: %+v", user)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("abcd1234")); err != nil {
		t.Errorf("stored password hash does not match original password: %v", err)
	}

	if repo.created == nil {
		t.Error("expected repository.Create to be called")
	}
}

func TestAuthService_Register_EmailTaken(t *testing.T) {
	repo := newFakeUserRepository()
	repo.byEmail["user@example.com"] = true
	svc := newTestAuthService(repo)

	_, err := svc.Register(context.Background(), "user@example.com", "john", "abcd1234")
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("Register() error = %v, want %v", err, domain.ErrEmailTaken)
	}
}

func TestAuthService_Register_TagTaken(t *testing.T) {
	repo := newFakeUserRepository()
	repo.byTag["null_pointer"] = true
	svc := newTestAuthService(repo)

	_, err := svc.Register(context.Background(), "user@example.com", "null_pointer", "abcd1234")
	if !errors.Is(err, domain.ErrTagTaken) {
		t.Errorf("Register() error = %v, want %v", err, domain.ErrTagTaken)
	}
}

func TestAuthService_Register_InvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		tag      string
		password string
		wantErr  error
	}{
		{"invalid email", "not-an-email", "john_doe", "abcd1234", domain.ErrInvalidEmail},
		{"invalid tag", "user@example.com", "j", "abcd1234", domain.ErrInvalidTag},
		{"weak password", "user@example.com", "john_doe", "weak", domain.ErrWeakPassword},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeUserRepository()
			svc := newTestAuthService(repo)

			_, err := svc.Register(context.Background(), tc.email, tc.tag, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Register() error = %v, want %v", err, tc.wantErr)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on invalid input")
			}
		})
	}
}
