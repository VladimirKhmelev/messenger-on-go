package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mediav1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/media/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/service"
)

type MediaServer struct {
	mediav1.UnimplementedMediaServiceServer

	media *service.MediaService
}

func NewMediaServer(media *service.MediaService) *MediaServer {
	return &MediaServer{media: media}
}

func (s *MediaServer) Health(ctx context.Context, req *mediav1.HealthRequest) (*mediav1.HealthResponse, error) {
	return &mediav1.HealthResponse{Ok: true}, nil
}

func (s *MediaServer) RequestUpload(ctx context.Context, req *mediav1.RequestUploadRequest) (*mediav1.RequestUploadResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	ticket, err := s.media.RequestUpload(ctx, req.GetChatId(), requesterID, req.GetContentType(), req.GetSizeBytes())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &mediav1.RequestUploadResponse{
		UploadId:      ticket.UploadID,
		UploadUrl:     ticket.UploadURL,
		ExpiresAtUnix: ticket.ExpiresAtUnix,
	}, nil
}

func (s *MediaServer) ConfirmUpload(ctx context.Context, req *mediav1.ConfirmUploadRequest) (*mediav1.ConfirmUploadResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	mediaID, err := s.media.ConfirmUpload(ctx, req.GetUploadId(), requesterID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &mediav1.ConfirmUploadResponse{MediaId: mediaID}, nil
}

func (s *MediaServer) GetDownloadURL(ctx context.Context, req *mediav1.GetDownloadURLRequest) (*mediav1.GetDownloadURLResponse, error) {
	requesterID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authenticated user")
	}

	ticket, err := s.media.GetDownloadURL(ctx, req.GetMediaId(), requesterID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &mediav1.GetDownloadURLResponse{
		DownloadUrl:   ticket.DownloadURL,
		ExpiresAtUnix: ticket.ExpiresAtUnix,
	}, nil
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrEmptyContentType),
		errors.Is(err, domain.ErrInvalidSize),
		errors.Is(err, domain.ErrSizeTooLarge),
		errors.Is(err, domain.ErrContentTypeBlocked):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrMediaNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrNotChatMember):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrUploadNotConfirmed):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
