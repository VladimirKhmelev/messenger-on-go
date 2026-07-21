package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

// publicMethods lists the full gRPC method names that do not require an
// access token. Everything else is protected by default.
var publicMethods = map[string]bool{
	"/auth.v1.AuthService/Health":       true,
	"/auth.v1.AuthService/Register":     true,
	"/auth.v1.AuthService/Login":        true,
	"/auth.v1.AuthService/RefreshToken": true,
	"/grpc.health.v1.Health/Check":      true,
	"/grpc.health.v1.Health/Watch":      true,
}

func AuthInterceptor(issuer *jwtutil.Issuer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		token, err := bearerTokenFromContext(ctx)
		if err != nil {
			return nil, err
		}

		claims, err := issuer.Parse(token, jwtutil.TokenTypeAccess)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		ctx = context.WithValue(ctx, userIDContextKey, claims.UserID)
		return handler(ctx, req)
	}
}

func bearerTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(values[0], prefix) {
		return "", status.Error(codes.Unauthenticated, "authorization header must use Bearer scheme")
	}

	return strings.TrimPrefix(values[0], prefix), nil
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}
