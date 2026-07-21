package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

func noopHandler(ctx context.Context, req any) (any, error) {
	return ctx, nil
}

func TestAuthInterceptor_PublicMethod_NoTokenRequired(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/Login"}

	_, err := interceptor(context.Background(), nil, info, noopHandler)
	if err != nil {
		t.Errorf("interceptor() unexpected error for public method: %v", err)
	}
}

func TestAuthInterceptor_ProtectedMethod_MissingToken(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/SearchUsers"}

	_, err := interceptor(context.Background(), nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_ProtectedMethod_ValidToken(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	accessToken, err := issuer.IssueAccessToken("user-1")
	if err != nil {
		t.Fatalf("IssueAccessToken() unexpected error: %v", err)
	}

	md := metadata.New(map[string]string{"authorization": "Bearer " + accessToken})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/SearchUsers"}

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
}

func TestAuthInterceptor_ProtectedMethod_InvalidToken(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	md := metadata.New(map[string]string{"authorization": "Bearer not-a-real-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/SearchUsers"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_ProtectedMethod_WrongScheme(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	md := metadata.New(map[string]string{"authorization": "Basic dXNlcjpwYXNz"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/SearchUsers"}

	_, err := interceptor(ctx, nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_ProtectedMethod_AccessTokenRejectsRefreshToken(t *testing.T) {
	issuer := jwtutil.NewIssuer("test-secret")
	interceptor := AuthInterceptor(issuer)

	refreshToken, err := issuer.IssueRefreshToken("user-1")
	if err != nil {
		t.Fatalf("IssueRefreshToken() unexpected error: %v", err)
	}

	md := metadata.New(map[string]string{"authorization": "Bearer " + refreshToken})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/auth.v1.AuthService/SearchUsers"}

	_, err = interceptor(ctx, nil, info, noopHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("interceptor() error code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}
