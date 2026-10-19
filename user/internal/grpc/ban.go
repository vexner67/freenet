package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type BanService interface {
	Ban(ctx context.Context, adminID, userID uuid.UUID, reason user.BanReason, until time.Time) error
	Unban(ctx context.Context, adminID, userID uuid.UUID) error
}

func (h *UserHandler) BanUser(ctx context.Context, req *userv1.BanUserRequest) (*emptypb.Empty, error) {
	adminID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	reason, err := user.NewBanReason(req.GetReason())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var until time.Time
	if req.GetUntil() != nil {
		until = req.GetUntil().AsTime()
	}

	if err = h.bans.Ban(ctx, adminID, userID, reason, until); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) UnbanUser(ctx context.Context, req *userv1.UnbanUserRequest) (*emptypb.Empty, error) {
	adminID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err = h.bans.Unban(ctx, adminID, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
