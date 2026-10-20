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
	Casino(ctx context.Context) (user.Casino, user.CasinoBets, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.CasinoSettings, error)
	SetCasino(ctx context.Context, adminID uuid.UUID, casino user.Casino) error
	Spin(ctx context.Context, userID uuid.UUID) (service.Spin, error)
	SetBets(ctx context.Context, adminID uuid.UUID, bets user.CasinoBets) error
	Bet(ctx context.Context, userID uuid.UUID, amount int64, multiplier int) (service.Wager, error)
}

func (h *UserHandler) GetCasino(ctx context.Context, _ *userv1.GetCasinoRequest) (*userv1.GetCasinoResponse, error) {
	casino, bets, err := h.casinos.Casino(ctx)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetCasinoResponse{
		Price:     casino.Price(),
		BetLeast:  bets.Least(),
		BetMost:   bets.Most(),
		BetReturn: int32(bets.Return()), //nolint:gosec // at most user.MaxBetReturn
	}
	for _, p := range casino.Prizes() {
		resp.Prizes = append(resp.Prizes, p.Votes)
	}
	resp.Multipliers = toProtoMultipliers()

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
		Price:       settings.Casino.Price(),
		Prizes:      toProtoCasinoPrizes(settings.Casino.Prizes()),
		Spins:       settings.Stats.Spins,
		Spent:       settings.Stats.Spent,
		Won:         settings.Stats.Won,
		BetLeast:    settings.Bets.Least(),
		BetMost:     settings.Bets.Most(),
		BetReturn:   int32(settings.Bets.Return()), //nolint:gosec // at most user.MaxBetReturn
		Bets:        settings.Stats.Wagers,
		BetSpent:    settings.Stats.WagerSpent,
		BetWon:      settings.Stats.WagerWon,
		Multipliers: toProtoMultipliers(),
	}, nil
}

// toProtoMultipliers are user.BetMultipliers, which the casino does not
// keep: they are the same for every casino.
func toProtoMultipliers() []int32 {
	multipliers := make([]int32, 0, len(user.BetMultipliers))
	for _, m := range user.BetMultipliers {
		multipliers = append(multipliers, int32(m)) //nolint:gosec // a few hundred
	}

	return multipliers
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

func (h *UserHandler) SetCasinoBets(ctx context.Context, req *userv1.SetCasinoBetsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	bets, err := user.NewCasinoBets(req.GetLeast(), req.GetMost(), int(req.GetReturn()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.casinos.SetBets(ctx, adminID, bets); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) BetCasino(ctx context.Context, req *userv1.BetCasinoRequest) (*userv1.BetCasinoResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	wager, err := h.casinos.Bet(ctx, userID, req.GetAmount(), int(req.GetMultiplier()))
	if err != nil {
		return nil, err
	}

	return &userv1.BetCasinoResponse{
		Won:     wager.Won,
		Roll:    int32(wager.Roll),   //nolint:gosec // below user.BetOdds
		Chance:  int32(wager.Chance), //nolint:gosec // at most user.BetOdds
		Votes:   wager.Votes,
		Balance: wager.Balance,
	}, nil
}
