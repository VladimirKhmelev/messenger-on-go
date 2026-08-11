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

	auth         *service.AuthService
	cookieSecure bool
}

func NewAuthServer(auth *service.AuthService, cookieSecure bool) *AuthServer {
	return &AuthServer{auth: auth, cookieSecure: cookieSecure}
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

	if err := setRefreshCookie(ctx, tokens.RefreshToken, s.cookieSecure); err != nil {
		return nil, status.Error(codes.Internal, "failed to set refresh cookie")
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

func (s *AuthServer) GetUserByID(ctx context.Context, req *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error) {
	user, err := s.auth.GetUserByID(ctx, req.GetUserId())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.GetUserByIDResponse{
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
	refreshToken := req.GetRefreshToken()
	if cookieToken, err := refreshTokenFromCookie(ctx); err == nil {
		refreshToken = cookieToken
	}

	tokens, err := s.auth.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, toGRPCError(err)
	}

	if err := setRefreshCookie(ctx, tokens.RefreshToken, s.cookieSecure); err != nil {
		return nil, status.Error(codes.Internal, "failed to set refresh cookie")
	}

	return &authv1.RefreshTokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}, nil
}

func (s *AuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	refreshToken := req.GetRefreshToken()
	if cookieToken, err := refreshTokenFromCookie(ctx); err == nil {
		refreshToken = cookieToken
	}

	if err := s.auth.Logout(ctx, refreshToken); err != nil {
		return nil, toGRPCError(err)
	}

	if err := clearRefreshCookie(ctx, s.cookieSecure); err != nil {
		return nil, status.Error(codes.Internal, "failed to clear refresh cookie")
	}

	return &authv1.LogoutResponse{}, nil
}

func (s *AuthServer) VerifyEmail(ctx context.Context, req *authv1.VerifyEmailRequest) (*authv1.VerifyEmailResponse, error) {
	if err := s.auth.VerifyEmail(ctx, req.GetEmail(), req.GetCode()); err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.VerifyEmailResponse{}, nil
}

func (s *AuthServer) RequestPasswordReset(ctx context.Context, req *authv1.RequestPasswordResetRequest) (*authv1.RequestPasswordResetResponse, error) {
	if err := s.auth.RequestPasswordReset(ctx, req.GetEmail()); err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.RequestPasswordResetResponse{}, nil
}

func (s *AuthServer) ResetPassword(ctx context.Context, req *authv1.ResetPasswordRequest) (*authv1.ResetPasswordResponse, error) {
	if err := s.auth.ResetPassword(ctx, req.GetToken(), req.GetNewPassword()); err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.ResetPasswordResponse{}, nil
}

func (s *AuthServer) LoginWithGitHub(ctx context.Context, req *authv1.LoginWithGitHubRequest) (*authv1.LoginWithGitHubResponse, error) {
	tokens, err := s.auth.LoginWithGitHub(ctx, req.GetCode())
	if err != nil {
		return nil, toGRPCError(err)
	}

	if err := setRefreshCookie(ctx, tokens.RefreshToken, s.cookieSecure); err != nil {
		return nil, status.Error(codes.Internal, "failed to set refresh cookie")
	}

	return &authv1.LoginWithGitHubResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}, nil
}

func (s *AuthServer) UpdateTag(ctx context.Context, req *authv1.UpdateTagRequest) (*authv1.UpdateTagResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	if err := s.auth.UpdateTag(ctx, userID, req.GetTag()); err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.UpdateTagResponse{Tag: req.GetTag()}, nil
}

func (s *AuthServer) CheckTagAvailable(ctx context.Context, req *authv1.CheckTagAvailableRequest) (*authv1.CheckTagAvailableResponse, error) {
	available, suggested, err := s.auth.CheckTagAvailable(ctx, req.GetTag())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &authv1.CheckTagAvailableResponse{Available: available, SuggestedTag: suggested}, nil
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
		errors.Is(err, domain.ErrInvalidToken),
		errors.Is(err, domain.ErrEmailNotVerified),
		errors.Is(err, domain.ErrInvalidVerificationCode),
		errors.Is(err, domain.ErrOAuthNoVerifiedEmail):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrTooManyAttempts):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
