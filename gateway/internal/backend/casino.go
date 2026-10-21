package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Casino is the casino of the site, which the user service keeps with the
// votes.
type Casino struct {
	api userv1.UserServiceClient
}

func NewCasino(api userv1.UserServiceClient) *Casino {
	return &Casino{
		api: api,
	}
}

func (c *Casino) Get(ctx context.Context) (model.Casino, error) {
	resp, err := c.api.GetCasino(ctx, &userv1.GetCasinoRequest{})
	if err != nil {
		return model.Casino{}, errs.Wrap("c.api.GetCasino", fromStatus(err))
	}

	casino := model.Casino{Price: resp.GetPrice(), Bets: model.CasinoBets{
		Least:       resp.GetBetLeast(),
		Most:        resp.GetBetMost(),
		Multipliers: fromProtoAim(resp.GetAim()),
	}, Rocket: fromProtoRocket(resp.GetRocket()), Jackpot: resp.GetJackpot()}
	for _, votes := range resp.GetPrizes() {
		casino.Prizes = append(casino.Prizes, model.CasinoPrize{Votes: votes})
	}

	return casino, nil
}

func (c *Casino) Settings(ctx context.Context, accessToken string) (model.CasinoSettings, error) {
	resp, err := c.api.GetCasinoSettings(withToken(ctx, accessToken), &userv1.GetCasinoSettingsRequest{})
	if err != nil {
		return model.CasinoSettings{}, errs.Wrap("c.api.GetCasinoSettings", fromStatus(err))
	}

	settings := model.CasinoSettings{
		Casino: model.Casino{Price: resp.GetPrice(), Bets: model.CasinoBets{
			Least:       resp.GetBetLeast(),
			Most:        resp.GetBetMost(),
			Multipliers: fromProtoAim(resp.GetAim()),
		}, Rocket: fromProtoRocket(resp.GetRocket()), Jackpot: resp.GetJackpot()},
		Spins:       resp.GetSpins(),
		Spent:       resp.GetSpent(),
		Won:         resp.GetWon(),
		Wagers:      resp.GetBets(),
		WagerSpent:  resp.GetBetSpent(),
		WagerWon:    resp.GetBetWon(),
		Rockets:     resp.GetRockets(),
		RocketSpent: resp.GetRocketSpent(),
		RocketWon:   resp.GetRocketWon(),
	}
	for _, p := range resp.GetPrizes() {
		settings.Casino.Prizes = append(settings.Casino.Prizes, model.CasinoPrize{Votes: p.GetVotes(), Weight: int(p.GetWeight())})
	}

	return settings, nil
}

func (c *Casino) Set(ctx context.Context, accessToken string, casino model.Casino) error {
	req := &userv1.SetCasinoRequest{Price: casino.Price}
	for _, p := range casino.Prizes {
		if p.Weight < 0 || p.Weight > math.MaxInt32 {
			return failure.New(failure.InvalidArgument, "the weight of a prize must be from 0 to 1000000")
		}

		req.Prizes = append(req.Prizes, &userv1.CasinoPrize{Votes: p.Votes, Weight: int32(p.Weight)})
	}

	if _, err := c.api.SetCasino(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("c.api.SetCasino", fromStatus(err))
	}

	return nil
}

func (c *Casino) Spin(ctx context.Context, accessToken string) (model.CasinoSpin, error) {
	resp, err := c.api.SpinCasino(withToken(ctx, accessToken), &userv1.SpinCasinoRequest{})
	if err != nil {
		return model.CasinoSpin{}, errs.Wrap("c.api.SpinCasino", fromStatus(err))
	}

	return model.CasinoSpin{Prize: int(resp.GetPrize()), Votes: resp.GetVotes(), Balance: resp.GetBalance()}, nil
}

func (c *Casino) SetBets(ctx context.Context, accessToken string, bets model.CasinoBets) error {
	req := &userv1.SetCasinoBetsRequest{Least: bets.Least, Most: bets.Most}
	for _, m := range bets.Multipliers {
		if m.Multiplier < 0 || m.Multiplier > math.MaxInt32 {
			return failure.New(failure.InvalidArgument, "a multiplier must be from 1.01 to 1000")
		}
		if m.Chance < 0 || m.Chance > math.MaxInt32 {
			return failure.New(failure.InvalidArgument, "the chance of a multiplier must be from 0.01 to 99.99 percent")
		}

		req.Aim = append(req.Aim, &userv1.AimMultiplier{Multiplier: int32(m.Multiplier), Chance: int32(m.Chance)})
	}

	if _, err := c.api.SetCasinoBets(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("c.api.SetCasinoBets", fromStatus(err))
	}

	return nil
}

func (c *Casino) Bet(ctx context.Context, accessToken string, amount int64, multiplier int) (model.CasinoWager, error) {
	if multiplier < 0 || multiplier > math.MaxInt32 {
		return model.CasinoWager{}, failure.New(failure.InvalidArgument, "unknown multiplier")
	}

	resp, err := c.api.BetCasino(withToken(ctx, accessToken), &userv1.BetCasinoRequest{Amount: amount, Multiplier: int32(multiplier)})
	if err != nil {
		return model.CasinoWager{}, errs.Wrap("c.api.BetCasino", fromStatus(err))
	}

	return model.CasinoWager{
		Won:     resp.GetWon(),
		Roll:    int(resp.GetRoll()),
		Chance:  int(resp.GetChance()),
		Votes:   resp.GetVotes(),
		Balance: resp.GetBalance(),
	}, nil
}

func (c *Casino) SetRocket(ctx context.Context, accessToken string, rocket model.CasinoRocket) error {
	if rocket.Return < 0 || rocket.Return > math.MaxInt32 {
		return failure.New(failure.InvalidArgument, "the return of the rocket must be from 0.01 to 100 percent")
	}
	if rocket.Top < 0 || rocket.Top > math.MaxInt32 {
		return failure.New(failure.InvalidArgument, "the top of the rocket must be from 2 to 10000")
	}

	req := &userv1.SetCasinoRocketRequest{Rocket: &userv1.CasinoRocket{
		Least:  rocket.Least,
		Most:   rocket.Most,
		Return: int32(rocket.Return),
		Top:    int32(rocket.Top),
	}}
	if _, err := c.api.SetCasinoRocket(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("c.api.SetCasinoRocket", fromStatus(err))
	}

	return nil
}

// LaunchRocket bets amount votes on a rocket cashing by itself at auto
// (0 — by hand).
func (c *Casino) LaunchRocket(ctx context.Context, accessToken string, amount int64, auto int) (model.RocketRound, error) {
	if auto < 0 || auto > math.MaxInt32 {
		return model.RocketRound{}, failure.New(failure.InvalidArgument, "the rocket cashes by itself from 1.01 to its top")
	}

	resp, err := c.api.LaunchRocket(withToken(ctx, accessToken), &userv1.LaunchRocketRequest{Amount: amount, Auto: int32(auto)})
	if err != nil {
		return model.RocketRound{}, errs.Wrap("c.api.LaunchRocket", fromStatus(err))
	}

	return fromProtoRocketRound(resp), nil
}

func (c *Casino) Rocket(ctx context.Context, accessToken, id string) (model.RocketRound, error) {
	resp, err := c.api.GetRocket(withToken(ctx, accessToken), &userv1.RocketRequest{RocketId: id})
	if err != nil {
		return model.RocketRound{}, errs.Wrap("c.api.GetRocket", fromStatus(err))
	}

	return fromProtoRocketRound(resp), nil
}

func (c *Casino) CashRocket(ctx context.Context, accessToken, id string) (model.RocketRound, error) {
	resp, err := c.api.CashRocket(withToken(ctx, accessToken), &userv1.RocketRequest{RocketId: id})
	if err != nil {
		return model.RocketRound{}, errs.Wrap("c.api.CashRocket", fromStatus(err))
	}

	return fromProtoRocketRound(resp), nil
}

func fromProtoRocket(r *userv1.CasinoRocket) model.CasinoRocket {
	return model.CasinoRocket{Least: r.GetLeast(), Most: r.GetMost(), Return: int(r.GetReturn()), Top: int(r.GetTop())}
}

var rocketStates = map[userv1.RocketState]model.RocketState{
	userv1.RocketState_ROCKET_STATE_FLYING:  model.RocketFlying,
	userv1.RocketState_ROCKET_STATE_CASHED:  model.RocketCashed,
	userv1.RocketState_ROCKET_STATE_CRASHED: model.RocketCrashed,
}

func fromProtoRocketRound(r *userv1.RocketRound) model.RocketRound {
	return model.RocketRound{
		ID:        r.GetId(),
		Amount:    r.GetAmount(),
		Auto:      int(r.GetAuto()),
		State:     rocketStates[r.GetState()],
		Cashed:    int(r.GetCashed()),
		Payout:    r.GetPayout(),
		Crash:     int(r.GetCrash()),
		ElapsedMS: r.GetElapsedMs(),
		Balance:   r.GetBalance(),
	}
}

// fromProtoAim are the multipliers of the bets with their chances.
func fromProtoAim(aim []*userv1.AimMultiplier) []model.AimMultiplier {
	out := make([]model.AimMultiplier, 0, len(aim))
	for _, m := range aim {
		out = append(out, model.AimMultiplier{Multiplier: int(m.GetMultiplier()), Chance: int(m.GetChance())})
	}

	return out
}

func (c *Casino) SetJackpot(ctx context.Context, accessToken string, jackpot int64) error {
	if _, err := c.api.SetCasinoJackpot(withToken(ctx, accessToken), &userv1.SetCasinoJackpotRequest{Jackpot: jackpot}); err != nil {
		return errs.Wrap("c.api.SetCasinoJackpot", fromStatus(err))
	}

	return nil
}
