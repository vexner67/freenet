package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Prime is Freenet Prime, the subscription of the site the user service
// sells for votes.
type Prime struct {
	api userv1.UserServiceClient
}

func NewPrime(api userv1.UserServiceClient) *Prime {
	return &Prime{
		api: api,
	}
}

func (p *Prime) Prices(ctx context.Context) (model.PrimePrices, error) {
	resp, err := p.api.GetPrime(ctx, &userv1.GetPrimeRequest{})
	if err != nil {
		return model.PrimePrices{}, errs.Wrap("p.api.GetPrime", fromStatus(err))
	}

	prices := resp.GetPrices()

	return model.PrimePrices{Month: prices.GetMonth(), Quarter: prices.GetQuarter(), Year: prices.GetYear()}, nil
}

func (p *Prime) SetPrices(ctx context.Context, accessToken string, prices model.PrimePrices) error {
	if _, err := p.api.SetPrimePrices(withToken(ctx, accessToken), &userv1.SetPrimePricesRequest{
		Prices: &userv1.PrimePrices{Month: prices.Month, Quarter: prices.Quarter, Year: prices.Year},
	}); err != nil {
		return errs.Wrap("p.api.SetPrimePrices", fromStatus(err))
	}

	return nil
}

// Buy buys period of Prime for userID, "" for the caller.
func (p *Prime) Buy(ctx context.Context, accessToken, userID, period string) (model.PrimePurchase, error) {
	resp, err := p.api.BuyPrime(withToken(ctx, accessToken), &userv1.BuyPrimeRequest{Period: period, UserId: userID})
	if err != nil {
		// Why a present is refused is the user service's to say, as with
		// gifts.
		return model.PrimePurchase{}, errs.Wrap("p.api.BuyPrime", giftRefusal(err))
	}

	return model.PrimePurchase{Until: fromTimestamp(resp.GetUntil()), Balance: resp.GetBalance()}, nil
}

func (p *Prime) SetInvisible(ctx context.Context, accessToken string, invisible bool) error {
	if _, err := p.api.SetPrimeInvisible(withToken(ctx, accessToken), &userv1.SetPrimeInvisibleRequest{Invisible: invisible}); err != nil {
		return errs.Wrap("p.api.SetPrimeInvisible", fromStatus(err))
	}

	return nil
}
