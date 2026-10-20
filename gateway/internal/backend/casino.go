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

	casino := model.Casino{Price: resp.GetPrice()}
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
		Casino: model.Casino{Price: resp.GetPrice()},
		Spins:  resp.GetSpins(),
		Spent:  resp.GetSpent(),
		Won:    resp.GetWon(),
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
