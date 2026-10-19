package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AccountService interface {
	Deactivate(ctx context.Context, id uuid.UUID, reason user.DeactivationReason) (*user.User, error)
	Reactivate(ctx context.Context, id uuid.UUID) (*user.User, error)
	State(ctx context.Context, id uuid.UUID) (service.AccountState, error)
}

func (h *UserHandler) DeactivateAccount(
	ctx context.Context,
	req *userv1.DeactivateAccountRequest,
) (*userv1.DeactivateAccountResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	reason, err := user.NewDeactivationReason(req.GetReason())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.accountService.Deactivate(ctx, userID, reason)
	if err != nil {
		return nil, err
	}

	return &userv1.DeactivateAccountResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) ReactivateAccount(
	ctx context.Context,
	_ *userv1.ReactivateAccountRequest,
) (*userv1.ReactivateAccountResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	u, err := h.accountService.Reactivate(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.ReactivateAccountResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) GetAccountState(
	ctx context.Context,
	req *userv1.GetAccountStateRequest,
) (*userv1.GetAccountStateResponse, error) {
	id, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	state, err := h.accountService.State(ctx, id)
	if err != nil {
		return nil, err
	}

	return &userv1.GetAccountStateResponse{Deactivated: state.Deactivated, Banned: state.Banned}, nil
}
