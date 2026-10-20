package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type CasinoService interface {
	Casino(ctx context.Context) (user.Casino, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.CasinoSettings, error)
	SetCasino(ctx context.Context, adminID uuid.UUID, casino user.Casino) error
	Spin(ctx context.Context, userID uuid.UUID) (service.Spin, error)
}

func (h *UserHandler) GetCasino(ctx context.Context, _ *userv1.GetCasinoRequest) (*userv1.GetCasinoResponse, error) {
	casino, err := h.casinos.Casino(ctx)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetCasinoResponse{Price: casino.Price()}
	for _, p := range casino.Prizes() {
		resp.Prizes = append(resp.Prizes, p.Votes)
	}

	return resp, nil
}

func (h *UserHandler) GetCasinoSettings(ctx context.Context, _ *userv1.GetCasinoSettingsRequest) (*userv1.GetCasinoSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := h.casinos.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetCasinoSettingsResponse{
		Price:  settings.Casino.Price(),
		Prizes: toProtoCasinoPrizes(settings.Casino.Prizes()),
		Spins:  settings.Stats.Spins,
		Spent:  settings.Stats.Spent,
		Won:    settings.Stats.Won,
	}, nil
}

func (h *UserHandler) SetCasino(ctx context.Context, req *userv1.SetCasinoRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	casino, err := user.NewCasino(req.GetPrice(), fromProtoCasinoPrizes(req.GetPrizes()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.casinos.SetCasino(ctx, adminID, casino); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SpinCasino(ctx context.Context, _ *userv1.SpinCasinoRequest) (*userv1.SpinCasinoResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	spin, err := h.casinos.Spin(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.SpinCasinoResponse{
		Prize:   int32(spin.Prize), //nolint:gosec // there are at most user.MaxCasinoPrizes
		Votes:   spin.Votes,
		Balance: spin.Balance,
	}, nil
}
