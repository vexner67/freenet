package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RocketService interface {
	SetRocket(ctx context.Context, adminID uuid.UUID, rocket user.CasinoRocket) error
	Launch(ctx context.Context, userID uuid.UUID, amount int64, auto int) (service.RocketView, error)
	Round(ctx context.Context, userID, id uuid.UUID) (service.RocketView, error)
	Cash(ctx context.Context, userID, id uuid.UUID) (service.RocketView, error)
}

func toProtoRocket(r user.CasinoRocket) *userv1.CasinoRocket {
	return &userv1.CasinoRocket{
		Least:  r.Least(),
		Most:   r.Most(),
		Return: int32(r.Return()), //nolint:gosec // at most user.MaxRocketReturn
		Top:    int32(r.Top()),    //nolint:gosec // at most user.MaxRocketTop
	}
}

var rocketStates = map[user.RocketState]userv1.RocketState{
	user.RocketFlying:  userv1.RocketState_ROCKET_STATE_FLYING,
	user.RocketCashed:  userv1.RocketState_ROCKET_STATE_CASHED,
	user.RocketCrashed: userv1.RocketState_ROCKET_STATE_CRASHED,
}

func toProtoRocketRound(v service.RocketView, now time.Time) *userv1.RocketRound {
	r := v.Round

	return &userv1.RocketRound{
		Id:        r.ID().String(),
		Amount:    r.Amount(),
		Auto:      int32(r.Auto()), //nolint:gosec // at most user.MaxRocketTop
		State:     rocketStates[v.State],
		Cashed:    int32(r.Cashed()), //nolint:gosec // at most user.MaxRocketTop
		Payout:    r.Payout(),
		Crash:     int32(v.Crash()), //nolint:gosec // at most user.MaxRocketTop
		ElapsedMs: now.Sub(r.StartedAt()).Milliseconds(),
		Balance:   v.Balance,
	}
}

func (h *UserHandler) SetCasinoRocket(ctx context.Context, req *userv1.SetCasinoRocketRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	r := req.GetRocket()

	rocket, err := user.NewCasinoRocket(r.GetLeast(), r.GetMost(), int(r.GetReturn()), int(r.GetTop()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.rockets.SetRocket(ctx, adminID, rocket); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) LaunchRocket(ctx context.Context, req *userv1.LaunchRocketRequest) (*userv1.RocketRound, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := h.rockets.Launch(ctx, userID, req.GetAmount(), int(req.GetAuto()))
	if err != nil {
		return nil, err
	}

	return toProtoRocketRound(view, view.Round.StartedAt()), nil
}

func (h *UserHandler) GetRocket(ctx context.Context, req *userv1.RocketRequest) (*userv1.RocketRound, error) {
	return h.rocket(ctx, req, h.rockets.Round)
}

func (h *UserHandler) CashRocket(ctx context.Context, req *userv1.RocketRequest) (*userv1.RocketRound, error) {
	return h.rocket(ctx, req, h.rockets.Cash)
}

// rocket is a round of the caller read or cashed by do.
func (h *UserHandler) rocket(
	ctx context.Context,
	req *userv1.RocketRequest,
	do func(ctx context.Context, userID, id uuid.UUID) (service.RocketView, error),
) (*userv1.RocketRound, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetRocketId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid rocket id")
	}

	view, err := do(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return toProtoRocketRound(view, time.Now()), nil
}
