package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PresenceService interface {
	MarkOnline(ctx context.Context, id uuid.UUID) error
}

// MarkOnline: the caller is on the site now ("Онлайн").
func (h *UserHandler) MarkOnline(ctx context.Context, _ *userv1.MarkOnlineRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err = h.presenceService.MarkOnline(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
