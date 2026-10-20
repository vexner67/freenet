package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AlertService interface {
	Set(ctx context.Context, adminID, userID uuid.UUID, alert user.Alert) error
}

func (h *UserHandler) SetAlert(ctx context.Context, req *userv1.SetAlertRequest) (*emptypb.Empty, error) {
	adminID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	alert, err := user.NewAlert(req.GetAlert())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.alerts.Set(ctx, adminID, userID, alert); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
