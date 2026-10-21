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
	}}
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
		}},
		Spins:      resp.GetSpins(),
		Spent:      resp.GetSpent(),
		Won:        resp.GetWon(),
		Wagers:     resp.GetBets(),
		WagerSpent: resp.GetBetSpent(),
		WagerWon:   resp.GetBetWon(),
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

// fromProtoAim are the multipliers of the bets with their chances.
func fromProtoAim(aim []*userv1.AimMultiplier) []model.AimMultiplier {
	out := make([]model.AimMultiplier, 0, len(aim))
	for _, m := range aim {
		out = append(out, model.AimMultiplier{Multiplier: int(m.GetMultiplier()), Chance: int(m.GetChance())})
	}

	return out
}
