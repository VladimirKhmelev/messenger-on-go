package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	testJWTSecret      = "test-secret"
	testInternalSecret = "internal-secret"
)

func noopHandler(ctx context.Context, req any) (any, error) {
	return ctx, nil
}

type fakePresenceMarker struct {
	onlineUserIDs []string
	err           error
}

func (m *fakePresenceMarker) SetOnline(_ context.Context, userID string) error {
	if m.err != nil {
		return m.err
	}
	m.onlineUserIDs = append(m.onlineUserIDs, userID)
	return nil
}

type testClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

func issueTestAccessToken(t *testing.T, userID string) string {
	t.Helper()

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
		UserID: userID,
		Type:   "access",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func TestAuthInterceptor_PublicMethod_NoTokenRequired(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/Health"}

	_, err := interceptor(context.Background(), nil, info, noopHandler)
	if err != nil {
		t.Errorf("interceptor() unexpected error for public method: %v", err)
	}
}

func TestAuthInterceptor_ProtectedMethod_MissingToken(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}

	_, err := interceptor(context.Background(), nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_ProtectedMethod_ValidToken_MarksUserOnline(t *testing.T) {
	presence := &fakePresenceMarker{}
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, presence)

	accessToken := issueTestAccessToken(t, "user-1")

	md := metadata.New(map[string]string{"authorization": "Bearer " + accessToken})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}

	result, err := interceptor(ctx, nil, info, noopHandler)
	if err != nil {
		t.Fatalf("interceptor() unexpected error: %v", err)
	}

	gotCtx, ok := result.(context.Context)
	if !ok {
		t.Fatalf("handler did not receive a context")
	}

	userID, ok := UserIDFromContext(gotCtx)
	if !ok || userID != "user-1" {
		t.Errorf("UserIDFromContext() = %q, %v, want %q, true", userID, ok, "user-1")
	}

	if len(presence.onlineUserIDs) != 1 || presence.onlineUserIDs[0] != "user-1" {
		t.Errorf("presence.onlineUserIDs = %v, want [user-1]", presence.onlineUserIDs)
	}
}

func TestAuthInterceptor_ProtectedMethod_InvalidToken(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	md := metadata.New(map[string]string{"authorization": "Bearer not-a-real-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_ProtectedMethod_WrongScheme(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	md := metadata.New(map[string]string{"authorization": "Basic dXNlcjpwYXNz"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_PresenceFailureDoesNotFailRequest(t *testing.T) {
	presence := &fakePresenceMarker{err: errors.New("redis down")}
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, presence)

	accessToken := issueTestAccessToken(t, "user-1")

	md := metadata.New(map[string]string{"authorization": "Bearer " + accessToken})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if err != nil {
		t.Fatalf("interceptor() unexpected error: %v, want nil (presence failures must not fail the request)", err)
	}
}

func TestAuthInterceptor_InternalMethod_ValidSecret(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	md := metadata.New(map[string]string{"x-internal-secret": testInternalSecret})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/ListMembers"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if err != nil {
		t.Errorf("interceptor() unexpected error for valid internal secret: %v", err)
	}
}

func TestAuthInterceptor_InternalMethod_MissingSecret(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/ListMembers"}

	_, err := interceptor(context.Background(), nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_GetPresence_IsInternalMethod(t *testing.T) {
	interceptor := AuthInterceptor(testJWTSecret, testInternalSecret, &fakePresenceMarker{})

	md := metadata.New(map[string]string{"x-internal-secret": testInternalSecret})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/GetPresence"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if err != nil {
		t.Errorf("interceptor() unexpected error for GetPresence with valid internal secret: %v", err)
	}
}
