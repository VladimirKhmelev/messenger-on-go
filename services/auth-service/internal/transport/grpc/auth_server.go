package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/service"
)

type AuthServer struct {
	authv1.UnimplementedAuthServiceServer

	auth *service.AuthService
}

func NewAuthServer(auth *service.AuthService) *AuthServer {
	return &AuthServer{auth: auth}
}

func (s *AuthServer) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	user, err := s.auth.Register(ctx, req.GetEmail(), req.GetTag(), req.GetPassword())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.RegisterResponse{UserId: user.ID}, nil
}

func (s *AuthServer) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	tokens, err := s.auth.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}, nil
}

func (s *AuthServer) GetUserByTag(ctx context.Context, req *authv1.GetUserByTagRequest) (*authv1.GetUserByTagResponse, error) {
	user, err := s.auth.GetUserByTag(ctx, req.GetTag())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.GetUserByTagResponse{
		UserId: user.ID,
		Email:  user.Email,
		Tag:    user.Tag,
	}, nil
}

func (s *AuthServer) SearchUsers(ctx context.Context, req *authv1.SearchUsersRequest) (*authv1.SearchUsersResponse, error) {
	users, err := s.auth.SearchUsers(ctx, req.GetQuery())
	if err != nil {
		return nil, toGRPCError(err)
	}

	summaries := make([]*authv1.UserSummary, 0, len(users))
	for _, user := range users {
		summaries = append(summaries, &authv1.UserSummary{
			UserId: user.ID,
			Email:  user.Email,
			Tag:    user.Tag,
		})
	}

	return &authv1.SearchUsersResponse{Users: summaries}, nil
}

func (s *AuthServer) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	tokens, err := s.auth.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.RefreshTokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}, nil
}

func (s *AuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if err := s.auth.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.LogoutResponse{}, nil
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail),
		errors.Is(err, domain.ErrInvalidTag),
		errors.Is(err, domain.ErrWeakPassword),
		errors.Is(err, domain.ErrSearchQueryTooShort):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrEmailTaken),
		errors.Is(err, domain.ErrTagTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrInvalidToken):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrTooManyAttempts):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
