package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AvatarService interface {
	Upload(ctx context.Context, userID uuid.UUID, data []byte) (uuid.UUID, error)
	Get(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error)
	DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error
}

type MediaHandler struct {
	mediav1.UnimplementedMediaServiceServer
	avatars AvatarService
	logger  *slog.Logger
}

func NewMediaHandler(avatars AvatarService, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{
		avatars: avatars,
		logger:  logger,
	}
}

func (h *MediaHandler) UploadAvatar(ctx context.Context, req *mediav1.UploadAvatarRequest) (*mediav1.UploadAvatarResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.avatars.Upload(ctx, userID, req.GetImage())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadAvatarResponse{AvatarId: id.String()}, nil
}

func (h *MediaHandler) DeleteAvatars(ctx context.Context, req *mediav1.DeleteAvatarsRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var keep uuid.UUID
	if req.GetKeepAvatarId() != "" {
		if keep, err = parseID(req.GetKeepAvatarId(), "avatar id"); err != nil {
			return nil, err
		}
	}

	if err = h.avatars.DeleteExcept(ctx, userID, keep); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) GetAvatar(ctx context.Context, req *mediav1.GetAvatarRequest) (*mediav1.GetAvatarResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	avatarID, err := parseID(req.GetAvatarId(), "avatar id")
	if err != nil {
		return nil, err
	}

	size := avatar.Large
	if req.GetSize() == mediav1.AvatarSize_AVATAR_SIZE_SMALL {
		size = avatar.Small
	}

	data, err := h.avatars.Get(ctx, userID, avatarID, size)
	if err != nil {
		return nil, err
	}

	return &mediav1.GetAvatarResponse{Image: data}, nil
}

func parseID(s, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid "+what)
	}

	return id, nil
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
