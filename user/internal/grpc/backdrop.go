package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BackdropService interface {
	Set(ctx context.Context, id uuid.UUID, side user.BackdropSide, photoID uuid.UUID) (*user.User, uuid.UUID, error)
}

var backdropSides = map[userv1.BackdropSide]user.BackdropSide{
	userv1.BackdropSide_BACKDROP_SIDE_LEFT:  user.BackdropLeft,
	userv1.BackdropSide_BACKDROP_SIDE_RIGHT: user.BackdropRight,
}

// SetBackdrop is openvk's "Фон страницы" of the caller, one side at a time.
func (h *UserHandler) SetBackdrop(ctx context.Context, req *userv1.SetBackdropRequest) (*userv1.SetBackdropResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	side, ok := backdropSides[req.GetSide()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, user.ErrInvalidBackdropSide.Error())
	}

	var photoID uuid.UUID
	if req.GetPhotoId() != "" {
		if photoID, err = parseID(req.GetPhotoId(), "photo id"); err != nil {
			return nil, err
		}
	}

	u, previous, err := h.backdropService.Set(ctx, userID, side, photoID)
	if err != nil {
		return nil, err
	}

	return &userv1.SetBackdropResponse{User: toProtoUser(u), PreviousPhotoId: avatarString(previous)}, nil
}
