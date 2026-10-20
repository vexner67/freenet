package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ForeignAgentService interface {
	Set(ctx context.Context, adminID, userID uuid.UUID, agent bool) error
}

func (h *UserHandler) SetForeignAgent(ctx context.Context, req *userv1.SetForeignAgentRequest) (*emptypb.Empty, error) {
	adminID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err = h.foreignAgents.Set(ctx, adminID, userID, req.GetForeignAgent()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
