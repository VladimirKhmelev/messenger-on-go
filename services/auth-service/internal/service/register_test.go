package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func newTestAuthService(repo *fakeUserRepository) *AuthService {
	return NewAuthService(repo, jwtutil.NewIssuer("test-secret"))
}

type fakeUserRepository struct {
	byEmail map[string]bool
	byTag   map[string]bool
	users   map[string]*domain.User
	created *domain.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		byEmail: map[string]bool{},
		byTag:   map[string]bool{},
		users:   map[string]*domain.User{},
	}
}

func (r *fakeUserRepository) Create(_ context.Context, user *domain.User) error {
	r.created = user
	r.users[user.Email] = user
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
