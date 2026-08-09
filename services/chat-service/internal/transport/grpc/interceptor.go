package grpc

import (
	"context"
	"log"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil"
)

type PresenceMarker interface {
	SetOnline(ctx context.Context, userID string) error
}

type contextKey string

const userIDContextKey contextKey = "user_id"

var publicMethods = map[string]bool{
	"/chat.v1.ChatService/Health":  true,
	"/grpc.health.v1.Health/Check": true,
	"/grpc.health.v1.Health/Watch": true,
}

var internalMethods = map[string]bool{
	"/chat.v1.ChatService/ListMembers":  true,
	"/chat.v1.ChatService/GetMessage":   true,
	"/chat.v1.ChatService/GetPresence":  true,
	"/chat.v1.ChatService/SetOffline":   true,
	"/chat.v1.ChatService/ListContacts": true,
}

func AuthInterceptor(jwtSecret, internalSecret string, presence PresenceMarker) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		if internalMethods[info.FullMethod] {
			if err := checkInternalSecret(ctx, internalSecret); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}

		token, err := bearerTokenFromContext(ctx)
		if err != nil {
			return nil, err
		}

		userID, err := jwtutil.ValidateAccessToken(token, jwtSecret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		if err := presence.SetOnline(ctx, userID); err != nil {
			log.Printf("chat-service: failed to mark user %s online: %v", userID, err)
		}

		ctx = context.WithValue(ctx, userIDContextKey, userID)
		return handler(ctx, req)
	}
}

func checkInternalSecret(ctx context.Context, internalSecret string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("x-internal-secret")
	if len(values) == 0 || values[0] != internalSecret {
		return status.Error(codes.Unauthenticated, "invalid internal secret")
	}

	return nil
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

func BearerTokenFromIncomingContext(ctx context.Context) (string, error) {
	return bearerTokenFromContext(ctx)
}
