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
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/oauth"
)

func newTestAuthService(repo *fakeUserRepository) *AuthService {
	return NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())
}

type fakeEventPublisher struct {
	registeredEvents     []events.UserRegistered
	passwordResetEvents  []events.UserPasswordReset
	oauthLinkedEvents    []events.UserOAuthLinked
	profileUpdatedEvents []events.UserProfileUpdated
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) PublishUserRegistered(_ context.Context, event events.UserRegistered) error {
	p.registeredEvents = append(p.registeredEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishUserPasswordReset(_ context.Context, event events.UserPasswordReset) error {
	p.passwordResetEvents = append(p.passwordResetEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishUserOAuthLinked(_ context.Context, event events.UserOAuthLinked) error {
	p.oauthLinkedEvents = append(p.oauthLinkedEvents, event)
	return nil
}

func (p *fakeEventPublisher) PublishUserProfileUpdated(_ context.Context, event events.UserProfileUpdated) error {
	p.profileUpdatedEvents = append(p.profileUpdatedEvents, event)
	return nil
}

type fakeGitHubClient struct {
	profile *oauth.GitHubProfile
	err     error
}

func newFakeGitHubClient() *fakeGitHubClient {
	return &fakeGitHubClient{}
}

func (c *fakeGitHubClient) FetchProfile(_ string) (*oauth.GitHubProfile, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.profile != nil {
		return c.profile, nil
	}
	return &oauth.GitHubProfile{ID: 1, Login: "octocat", Email: "octocat@example.com"}, nil
}

type fakeEmailVerificationStore struct {
	codes map[string]string
}

func newFakeEmailVerificationStore() *fakeEmailVerificationStore {
	return &fakeEmailVerificationStore{codes: map[string]string{}}
}

func (s *fakeEmailVerificationStore) GenerateAndStore(_ context.Context, email string) (string, error) {
	code := "123456"
	s.codes[email] = code
	return code, nil
}

func (s *fakeEmailVerificationStore) Verify(_ context.Context, email, code string) (bool, error) {
	stored, ok := s.codes[email]
	if !ok || stored != code {
		return false, nil
	}
	delete(s.codes, email)
	return true, nil
}

type fakeMailer struct {
	sentTo             string
	sentCode           string
	sentResetTo        string
	sentResetToken     string
	sentPasswordChange string
}

func newFakeMailer() *fakeMailer {
	return &fakeMailer{}
}

func (m *fakeMailer) SendVerificationCode(to, code string) error {
	m.sentTo = to
	m.sentCode = code
	return nil
}

func (m *fakeMailer) SendPasswordResetToken(to, token string) error {
	m.sentResetTo = to
	m.sentResetToken = token
	return nil
}

func (m *fakeMailer) SendPasswordChanged(to string) error {
	m.sentPasswordChange = to
	return nil
}

type fakePasswordResetStore struct {
	tokens map[string]string
}

func newFakePasswordResetStore() *fakePasswordResetStore {
	return &fakePasswordResetStore{tokens: map[string]string{}}
}

func (s *fakePasswordResetStore) GenerateAndStore(_ context.Context, email string) (string, error) {
	token := "reset-token-" + email
	s.tokens[token] = email
	return token, nil
}

func (s *fakePasswordResetStore) Consume(_ context.Context, token string) (string, bool, error) {
	email, ok := s.tokens[token]
	if !ok {
		return "", false, nil
	}
	delete(s.tokens, token)
	return email, true, nil
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

type fakePasswordChangeTracker struct {
	changedAt map[string]time.Time
}

func newFakePasswordChangeTracker() *fakePasswordChangeTracker {
	return &fakePasswordChangeTracker{changedAt: map[string]time.Time{}}
}

func (t *fakePasswordChangeTracker) MarkChanged(_ context.Context, userID string, _ time.Duration) error {
	t.changedAt[userID] = time.Now()
	return nil
}

func (t *fakePasswordChangeTracker) ChangedAfter(_ context.Context, userID string, issuedAt time.Time) (bool, error) {
	changed, ok := t.changedAt[userID]
	if !ok {
		return false, nil
	}
	return changed.After(issuedAt), nil
}

type fakeUserRepository struct {
	byEmail    map[string]bool
	byTag      map[string]bool
	users      map[string]*domain.User
	usersByTag map[string]*domain.User
	created    *domain.User
	avatars    map[string]*domain.Avatar
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		byEmail:    map[string]bool{},
		byTag:      map[string]bool{},
		users:      map[string]*domain.User{},
		usersByTag: map[string]*domain.User{},
		avatars:    map[string]*domain.Avatar{},
	}
}

func (r *fakeUserRepository) Create(_ context.Context, user *domain.User) error {
	r.created = user
	r.users[user.Email] = user
	r.usersByTag[user.Tag] = user
	r.byEmail[user.Email] = true
	r.byTag[user.Tag] = true
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

func (r *fakeUserRepository) GetByID(_ context.Context, id string) (*domain.User, error) {
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, domain.ErrUserNotFound
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

func (r *fakeUserRepository) MarkEmailVerified(_ context.Context, userID string) error {
	for _, user := range r.users {
		if user.ID == userID {
			user.EmailVerified = true
		}
	}
	return nil
}

func (r *fakeUserRepository) UpdatePasswordHash(_ context.Context, userID, passwordHash string) error {
	for _, user := range r.users {
		if user.ID == userID {
			user.PasswordHash = passwordHash
		}
	}
	return nil
}

func (r *fakeUserRepository) UpdateTag(_ context.Context, userID, tag string) error {
	for _, user := range r.users {
		if user.ID == userID {
			delete(r.usersByTag, user.Tag)
			delete(r.byTag, user.Tag)
			user.Tag = tag
			r.usersByTag[tag] = user
			r.byTag[tag] = true
		}
	}
	return nil
}

func (r *fakeUserRepository) UpdateDisplayName(_ context.Context, userID, displayName string) error {
	for _, user := range r.users {
		if user.ID == userID {
			user.DisplayName = displayName
		}
	}
	return nil
}

func (r *fakeUserRepository) UpsertAvatar(_ context.Context, avatar *domain.Avatar) error {
	r.avatars[avatar.UserID] = avatar
	return nil
}

func (r *fakeUserRepository) GetAvatar(_ context.Context, userID string) (*domain.Avatar, error) {
	avatar, ok := r.avatars[userID]
	if !ok {
		return nil, domain.ErrAvatarNotFound
	}
	return avatar, nil
}

func (r *fakeUserRepository) DeleteAvatar(_ context.Context, userID string) error {
	delete(r.avatars, userID)
	return nil
}

func TestAuthService_Register_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if user.Email != "user@example.com" || user.Tag != "balbes" {
		t.Errorf("Register() returned unexpected user: %+v", user)
	}

	if user.EmailVerified {
		t.Error("Register() returned a user with EmailVerified = true, want false until VerifyEmail is called")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("abcd1234")); err != nil {
		t.Errorf("stored password hash does not match original password: %v", err)
	}

	if repo.created == nil {
		t.Error("expected repository.Create to be called")
	}
}

func TestAuthService_Register_SendsVerificationCode(t *testing.T) {
	repo := newFakeUserRepository()
	mailer := newFakeMailer()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), newFakeRateLimiter(), mailer, newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	_, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if mailer.sentTo != "user@example.com" {
		t.Errorf("Register() sent verification email to %q, want %q", mailer.sentTo, "user@example.com")
	}
	if mailer.sentCode == "" {
		t.Error("Register() did not send a verification code")
	}
}

func TestAuthService_VerifyEmail_Success(t *testing.T) {
	repo := newFakeUserRepository()
	emailCodes := newFakeEmailVerificationStore()
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), emailCodes, newFakeRateLimiter(), newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	code := emailCodes.codes["user@example.com"]

	if err := svc.VerifyEmail(context.Background(), "user@example.com", code); err != nil {
		t.Fatalf("VerifyEmail() unexpected error: %v", err)
	}

	if !user.EmailVerified {
		t.Error("VerifyEmail() did not mark the user as verified")
	}
}

func TestAuthService_VerifyEmail_WrongCode(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	if _, err := svc.Register(context.Background(), "user@example.com", "balbes", "Test User", "abcd1234"); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	err := svc.VerifyEmail(context.Background(), "user@example.com", "000000")
	if !errors.Is(err, domain.ErrInvalidVerificationCode) {
		t.Errorf("VerifyEmail() error = %v, want %v", err, domain.ErrInvalidVerificationCode)
	}
}

func TestAuthService_VerifyEmail_RateLimited(t *testing.T) {
	repo := newFakeUserRepository()
	limiter := newFakeRateLimiter()
	limiter.allow = false
	svc := NewAuthService(repo, jwtutil.NewIssuer("test-secret"), newFakeRateLimiter(), newFakeTokenBlacklist(), newFakeEmailVerificationStore(), limiter, newFakeMailer(), newFakePasswordResetStore(), newFakeGitHubClient(), newFakeEventPublisher(), newFakePasswordChangeTracker())

	err := svc.VerifyEmail(context.Background(), "user@example.com", "123456")
	if !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Errorf("VerifyEmail() error = %v, want %v", err, domain.ErrTooManyAttempts)
	}
}

func TestAuthService_Register_EmailTaken(t *testing.T) {
	repo := newFakeUserRepository()
	repo.byEmail["user@example.com"] = true
	svc := newTestAuthService(repo)

	_, err := svc.Register(context.Background(), "user@example.com", "john", "Test User", "abcd1234")
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("Register() error = %v, want %v", err, domain.ErrEmailTaken)
	}
}

func TestAuthService_Register_TagTaken(t *testing.T) {
	repo := newFakeUserRepository()
	repo.byTag["null_pointer"] = true
	svc := newTestAuthService(repo)

	_, err := svc.Register(context.Background(), "user@example.com", "null_pointer", "Test User", "abcd1234")
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

			_, err := svc.Register(context.Background(), tc.email, tc.tag, "Test User", tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Register() error = %v, want %v", err, tc.wantErr)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on invalid input")
			}
		})
	}
}
