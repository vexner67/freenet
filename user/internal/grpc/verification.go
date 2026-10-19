package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type VerificationService interface {
	Set(ctx context.Context, adminID, userID uuid.UUID, verified bool) error
}

func (h *UserHandler) SetVerified(ctx context.Context, req *userv1.SetVerifiedRequest) (*emptypb.Empty, error) {
	adminID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err = h.verification.Set(ctx, adminID, userID, req.GetVerified()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
