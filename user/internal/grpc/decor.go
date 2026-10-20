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

type DecorService interface {
	Decor(ctx context.Context) (user.Decor, bool, error)
	SetDecor(ctx context.Context, adminID uuid.UUID, decor user.Decor) error
}

func (h *UserHandler) GetDecor(ctx context.Context, _ *userv1.GetDecorRequest) (*userv1.GetDecorResponse, error) {
	decor, festive, err := h.decors.Decor(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetDecorResponse{Decor: toProtoDecor(decor), Festive: festive}, nil
}

func (h *UserHandler) SetDecor(ctx context.Context, req *userv1.SetDecorRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	decor, err := fromProtoDecor(req.GetDecor())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.decors.SetDecor(ctx, adminID, decor); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
