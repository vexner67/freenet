package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Business is the businesses of the pages, which the user service keeps
// with the votes.
type Business struct {
	api userv1.UserServiceClient
}

func NewBusiness(api userv1.UserServiceClient) *Business {
	return &Business{
		api: api,
	}
}

func (b *Business) Get(ctx context.Context, accessToken string) (model.Businesses, error) {
	resp, err := b.api.GetBusinesses(withToken(ctx, accessToken), &userv1.GetBusinessesRequest{})
	if err != nil {
		return model.Businesses{}, errs.Wrap("b.api.GetBusinesses", fromStatus(err))
	}

	return fromProtoBusinesses(resp), nil
}

func (b *Business) Buy(ctx context.Context, accessToken, kind string) (model.BusinessDone, error) {
	resp, err := b.api.BuyBusiness(withToken(ctx, accessToken), &userv1.BuyBusinessRequest{Kind: kind})
	if err != nil {
		return model.BusinessDone{}, errs.Wrap("b.api.BuyBusiness", fromStatus(err))
	}

	return fromProtoBusinessDone(resp), nil
}

func (b *Business) Run(ctx context.Context, accessToken string, act model.BusinessAct) (model.BusinessDone, error) {
	staff, err := int32Of(act.Staff, "the staff")
	if err != nil {
		return model.BusinessDone{}, err
	}

	resp, err := b.api.RunBusiness(withToken(ctx, accessToken), &userv1.RunBusinessRequest{
		Kind: act.Kind, Action: act.Action, Amount: act.Amount, Price: act.Price, Staff: staff,
	})
	if err != nil {
		return model.BusinessDone{}, errs.Wrap("b.api.RunBusiness", fromStatus(err))
	}

	return fromProtoBusinessDone(resp), nil
}

func (b *Business) Settings(ctx context.Context, accessToken string) (model.BusinessSettings, model.BusinessStats, error) {
	resp, err := b.api.GetBusinessSettings(withToken(ctx, accessToken), &userv1.GetBusinessSettingsRequest{})
	if err != nil {
		return model.BusinessSettings{}, model.BusinessStats{}, errs.Wrap("b.api.GetBusinessSettings", fromStatus(err))
	}

	stats := model.BusinessStats{Businesses: resp.GetBusinesses(), Spent: resp.GetSpent(), Paid: resp.GetPaid()}

	return fromProtoBusinessSettings(resp.GetSettings()), stats, nil
}

func (b *Business) Set(ctx context.Context, accessToken string, settings model.BusinessSettings) error {
	req, err := toProtoBusinessSettings(settings)
	if err != nil {
		return err
	}

	if _, err = b.api.SetBusinessSettings(withToken(ctx, accessToken), &userv1.SetBusinessSettingsRequest{Settings: req}); err != nil {
		return errs.Wrap("b.api.SetBusinessSettings", fromStatus(err))
	}

	return nil
}

func fromProtoBusinessDone(resp *userv1.RunBusinessResponse) model.BusinessDone {
	return model.BusinessDone{
		Spent:      resp.GetSpent(),
		Got:        resp.GetGot(),
		Tax:        resp.GetTax(),
		Balance:    resp.GetBalance(),
		Businesses: fromProtoBusinesses(resp.GetBusinesses()),
	}
}

func fromProtoBusinesses(s *userv1.BusinessesState) model.Businesses {
	out := model.Businesses{Settings: fromProtoBusinessSettings(s.GetSettings())}
	for _, b := range s.GetBusinesses() {
		biz := model.Business{
			Kind: b.GetKind(), Level: int(b.GetLevel()), Stock: b.GetStock(), Price: b.GetPrice(),
			Staff: int(b.GetStaff()), Condition: int(b.GetCondition()), Cash: b.GetCash(),
			Bankrupt: b.GetBankrupt(), Invested: b.GetInvested(), Collected: b.GetCollected(),
			Customers: b.GetCustomers(), Served: b.GetServed(), Income: b.GetIncome(), Costs: b.GetCosts(),
			Resale: b.GetResale(), InDebt: b.GetInDebt(),
		}
		if b.GetAdUntil() != nil {
			biz.AdUntil = b.GetAdUntil().AsTime()
		}

		out.Businesses = append(out.Businesses, biz)
	}

	return out
}

func fromProtoBusinessSettings(s *userv1.BusinessSettings) model.BusinessSettings {
	out := model.BusinessSettings{Tax: int(s.GetTax())}
	for _, p := range s.GetPlans() {
		out.Plans = append(out.Plans, model.BusinessPlan{
			Kind: p.GetKind(), Price: p.GetPrice(), Upgrade: p.GetUpgrade(), Levels: int(p.GetLevels()),
			Demand: p.GetDemand(), Fair: p.GetFair(), Cost: p.GetCost(), Storage: p.GetStorage(),
			Serve: p.GetServe(), Staff: int(p.GetStaff()), Wage: p.GetWage(), Rent: p.GetRent(),
			Wear: int(p.GetWear()), Repair: p.GetRepair(), Till: p.GetTill(), Debt: p.GetDebt(),
			AdPrice: p.GetAdPrice(), AdHours: int(p.GetAdHours()), AdBoost: int(p.GetAdBoost()),
		})
	}

	return out
}

func toProtoBusinessSettings(s model.BusinessSettings) (*userv1.BusinessSettings, error) {
	tax, err := int32Of(s.Tax, "the tax")
	if err != nil {
		return nil, err
	}

	out := &userv1.BusinessSettings{Tax: tax}
	for _, p := range s.Plans {
		plan := &userv1.BusinessPlan{
			Kind: p.Kind, Price: p.Price, Upgrade: p.Upgrade, Demand: p.Demand, Fair: p.Fair, Cost: p.Cost,
			Storage: p.Storage, Serve: p.Serve, Wage: p.Wage, Rent: p.Rent, Repair: p.Repair,
			Till: p.Till, Debt: p.Debt, AdPrice: p.AdPrice,
		}

		ints := []struct {
			n    int
			what string
			to   *int32
		}{
			{p.Levels, "the levels", &plan.Levels},
			{p.Staff, "the staff", &plan.Staff},
			{p.Wear, "the wear", &plan.Wear},
			{p.AdHours, "the advertisement hours", &plan.AdHours},
			{p.AdBoost, "the advertisement boost", &plan.AdBoost},
		}
		for _, i := range ints {
			n, err := int32Of(i.n, i.what)
			if err != nil {
				return nil, err
			}

			*i.to = n
		}

		out.Plans = append(out.Plans, plan)
	}

	return out, nil
}
