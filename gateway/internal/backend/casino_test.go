package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeCasinoAPI struct {
	userv1.UserServiceClient
	md   metadata.MD
	set  *userv1.SetCasinoRequest
	bets *userv1.SetCasinoBetsRequest
	bet  *userv1.BetCasinoRequest
	// rocket is the last rocket set, launched and asked of.
	rocket *userv1.SetCasinoRocketRequest
	launch *userv1.LaunchRocketRequest
	asked  *userv1.RocketRequest
	err    error
}

func (f *fakeCasinoAPI) SetCasinoRocket(ctx context.Context, req *userv1.SetCasinoRocketRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.rocket = req

	return &emptypb.Empty{}, nil
}

func (f *fakeCasinoAPI) LaunchRocket(ctx context.Context, req *userv1.LaunchRocketRequest, _ ...grpc.CallOption) (*userv1.RocketRound, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.launch = req
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.RocketRound{Id: "r1", Amount: req.GetAmount(), State: userv1.RocketState_ROCKET_STATE_FLYING, ElapsedMs: 3, Balance: 90}, nil
}

func (f *fakeCasinoAPI) GetRocket(ctx context.Context, req *userv1.RocketRequest, _ ...grpc.CallOption) (*userv1.RocketRound, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.asked = req

	return &userv1.RocketRound{Id: req.GetRocketId(), Amount: 10, State: userv1.RocketState_ROCKET_STATE_CRASHED, Crash: 237, ElapsedMs: 9000}, nil
}

func (f *fakeCasinoAPI) CashRocket(ctx context.Context, req *userv1.RocketRequest, _ ...grpc.CallOption) (*userv1.RocketRound, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.asked = req

	return &userv1.RocketRound{Id: req.GetRocketId(), Amount: 10, State: userv1.RocketState_ROCKET_STATE_CASHED, Cashed: 200, Payout: 20, Crash: 237, Balance: 110}, nil
}

func (f *fakeCasinoAPI) SetCasinoBets(ctx context.Context, req *userv1.SetCasinoBetsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.bets = req

	return &emptypb.Empty{}, nil
}

func (f *fakeCasinoAPI) BetCasino(ctx context.Context, req *userv1.BetCasinoRequest, _ ...grpc.CallOption) (*userv1.BetCasinoResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.bet = req
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.BetCasinoResponse{Won: true, Roll: 12, Chance: 4750, Votes: 20, Balance: 30}, nil
}

func (f *fakeCasinoAPI) GetCasino(context.Context, *userv1.GetCasinoRequest, ...grpc.CallOption) (*userv1.GetCasinoResponse, error) {
	return &userv1.GetCasinoResponse{Price: 10, Prizes: []int64{0, 50}, BetLeast: 1, BetMost: 100, Aim: []*userv1.AimMultiplier{{Multiplier: 150, Chance: 6333}, {Multiplier: 200, Chance: 4750}},
		Rocket: &userv1.CasinoRocket{Least: 1, Most: 1000, Top: 100000}}, nil
}

func (f *fakeCasinoAPI) GetCasinoSettings(ctx context.Context, _ *userv1.GetCasinoSettingsRequest, _ ...grpc.CallOption) (*userv1.GetCasinoSettingsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)

	return &userv1.GetCasinoSettingsResponse{
		Price:  10,
		Prizes: []*userv1.CasinoPrize{{Votes: 0, Weight: 9}, {Votes: 50, Weight: 1}},
		Spins:  3, Spent: 30, Won: 50,
		BetLeast: 1, BetMost: 100, Aim: []*userv1.AimMultiplier{{Multiplier: 150, Chance: 6333}, {Multiplier: 200, Chance: 4750}, {Multiplier: 300, Chance: 3166}},
		Bets: 4, BetSpent: 40, BetWon: 20,
		Rocket:  &userv1.CasinoRocket{Least: 1, Most: 1000, Return: 9500, Top: 100000},
		Rockets: 5, RocketSpent: 50, RocketWon: 45,
	}, nil
}

func (f *fakeCasinoAPI) SetCasino(ctx context.Context, req *userv1.SetCasinoRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.set = req

	return &emptypb.Empty{}, nil
}

func (f *fakeCasinoAPI) SpinCasino(ctx context.Context, _ *userv1.SpinCasinoRequest, _ ...grpc.CallOption) (*userv1.SpinCasinoResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.SpinCasinoResponse{Prize: 1, Votes: 50, Balance: 60}, nil
}

func TestCasino(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeCasinoAPI{}
	c := NewCasino(api)

	if got, err := c.Get(ctx); err != nil || got.Price != 10 || len(got.Prizes) != 2 || got.Prizes[1].Votes != 50 ||
		got.Bets.Most != 100 || len(got.Bets.Multipliers) != 2 || got.Bets.Multipliers[1] != (model.AimMultiplier{Multiplier: 200, Chance: 4750}) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	settings, err := c.Settings(ctx, "tok")
	if err != nil || settings.Casino.Prizes[0].Weight != 9 || settings.Spins != 3 || settings.Won != 50 || api.md.Get("authorization")[0] != "Bearer tok" ||
		settings.Casino.Bets.Most != 100 || len(settings.Casino.Bets.Multipliers) != 3 || settings.Wagers != 4 || settings.WagerSpent != 40 || settings.WagerWon != 20 {
		t.Fatalf("Settings = %+v, %v, %v", settings, err, api.md)
	}

	if err = c.Set(ctx, "tok", model.Casino{Price: 5, Prizes: []model.CasinoPrize{{Votes: 0, Weight: 1}, {Votes: 7, Weight: 2}}}); err != nil ||
		api.set.GetPrice() != 5 || api.set.GetPrizes()[1].GetWeight() != 2 {
		t.Fatalf("Set: %v, %v", err, api.set)
	}

	if spin, err := c.Spin(ctx, "tok"); err != nil || spin != (model.CasinoSpin{Prize: 1, Votes: 50, Balance: 60}) {
		t.Fatalf("Spin = %+v, %v", spin, err)
	}

	api.err = status.Error(codes.FailedPrecondition, "not enough votes")
	if _, err = c.Spin(ctx, "tok"); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("Spin without votes: %v", err)
	}

	aim := []model.AimMultiplier{{Multiplier: 200, Chance: 4500}, {Multiplier: 900, Chance: 1000}}
	if err = c.SetBets(ctx, "tok", model.CasinoBets{Least: 2, Most: 20, Multipliers: aim}); err != nil ||
		api.bets.GetLeast() != 2 || api.bets.GetMost() != 20 || len(api.bets.GetAim()) != 2 || api.bets.GetAim()[1].GetChance() != 1000 {
		t.Fatalf("SetBets: %v, %v", err, api.bets)
	}
	if err = c.SetBets(ctx, "tok", model.CasinoBets{Least: 2, Most: 20, Multipliers: []model.AimMultiplier{{Multiplier: 200, Chance: -1}}}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("SetBets with a chance of -1: %v", err)
	}

	if _, err = c.Bet(ctx, "tok", 10, 200); failure.From(err).Kind != failure.Conflict || api.bet.GetAmount() != 10 || api.bet.GetMultiplier() != 200 {
		t.Fatalf("Bet without votes: %v, %v", err, api.bet)
	}

	api.err = nil
	if wager, err := c.Bet(ctx, "tok", 10, 200); err != nil || wager != (model.CasinoWager{Won: true, Roll: 12, Chance: 4750, Votes: 20, Balance: 30}) {
		t.Fatalf("Bet = %+v, %v", wager, err)
	}
}

func TestCasino_Rocket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeCasinoAPI{}
	c := NewCasino(api)

	if got, err := c.Get(ctx); err != nil || got.Rocket != (model.CasinoRocket{Least: 1, Most: 1000, Top: 100000}) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	settings, err := c.Settings(ctx, "tok")
	if err != nil || settings.Casino.Rocket.Return != 9500 || settings.Rockets != 5 || settings.RocketSpent != 50 || settings.RocketWon != 45 {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}

	if err = c.SetRocket(ctx, "tok", model.CasinoRocket{Least: 2, Most: 20, Return: 9000, Top: 5000}); err != nil ||
		api.rocket.GetRocket().GetReturn() != 9000 || api.rocket.GetRocket().GetTop() != 5000 || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("SetRocket: %v, %v", err, api.rocket)
	}
	if err = c.SetRocket(ctx, "tok", model.CasinoRocket{Least: 2, Most: 20, Return: -1, Top: 5000}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("SetRocket with a return of -1: %v", err)
	}

	round, err := c.LaunchRocket(ctx, "tok", 10, 250)
	if err != nil || round != (model.RocketRound{ID: "r1", Amount: 10, State: model.RocketFlying, ElapsedMS: 3, Balance: 90}) || api.launch.GetAuto() != 250 {
		t.Fatalf("LaunchRocket = %+v, %v", round, err)
	}
	if _, err = c.LaunchRocket(ctx, "tok", 10, -1); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("LaunchRocket at -1: %v", err)
	}

	if round, err = c.Rocket(ctx, "tok", "r1"); err != nil || round.State != model.RocketCrashed || round.Crash != 237 || api.asked.GetRocketId() != "r1" {
		t.Fatalf("Rocket = %+v, %v", round, err)
	}
	if round, err = c.CashRocket(ctx, "tok", "r1"); err != nil || round.State != model.RocketCashed || round.Payout != 20 || round.Balance != 110 {
		t.Fatalf("CashRocket = %+v, %v", round, err)
	}

	api.err = status.Error(codes.FailedPrecondition, "not enough votes")
	if _, err = c.LaunchRocket(ctx, "tok", 10, 0); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("LaunchRocket without votes: %v", err)
	}
}
