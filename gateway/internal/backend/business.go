package backend

import (
	"context"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	worker, err := int32Of(act.Worker, "the worker")
	if err != nil {
		return model.BusinessDone{}, err
	}

	resp, err := b.api.RunBusiness(withToken(ctx, accessToken), &userv1.RunBusinessRequest{
		Kind: act.Kind, Action: act.Action, Amount: act.Amount, Price: act.Price, Offer: act.Offer,
		Supply: act.Supply, Supplier: act.Supplier, Below: act.Below, Worker: worker,
		Candidate: act.Candidate, Option: act.Option,
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
		out.Businesses = append(out.Businesses, fromProtoBusiness(b))
	}
	for _, c := range s.GetCatalog() {
		out.Catalog = append(out.Catalog, fromProtoBusinessCatalog(c))
	}
	for _, p := range s.GetSuppliers() {
		out.Suppliers = append(out.Suppliers, model.BusinessSupplier{
			Supplier: p.GetSupplier(), Percent: p.GetPercent(), Hours: p.GetHours(), Short: int(p.GetShort()),
		})
	}

	return out
}

func fromProtoBusinessCatalog(c *userv1.BusinessCatalog) model.BusinessCatalog {
	out := model.BusinessCatalog{Kind: c.GetKind()}
	for _, s := range c.GetSupplies() {
		out.Supplies = append(out.Supplies, model.BusinessSupplySpec{
			Supply: s.GetSupply(), Cost: s.GetCost(), Spoil: int(s.GetSpoil()), Storage: s.GetStorage(),
		})
	}
	for _, o := range c.GetOffers() {
		offer := model.BusinessOfferSpec{Offer: o.GetOffer(), Profession: o.GetProfession(), Fair: o.GetFair(), Demand: o.GetDemand()}
		for _, i := range o.GetRecipe() {
			offer.Recipe = append(offer.Recipe, model.BusinessIngredient{Supply: i.GetSupply(), Units: i.GetUnits()})
		}

		out.Offers = append(out.Offers, offer)
	}
	for _, p := range c.GetProfessions() {
		out.Professions = append(out.Professions, model.BusinessProfessionSpec{
			Profession: p.GetProfession(), Wage: p.GetWage(), Serve: p.GetServe(),
		})
	}

	return out
}

// timeOf is the time of a timestamp, or zero without one.
func timeOf(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

func fromProtoBusiness(b *userv1.Business) model.Business {
	out := model.Business{
		Kind: b.GetKind(), Level: int(b.GetLevel()), Condition: int(b.GetCondition()), Cash: b.GetCash(),
		AdUntil: timeOf(b.GetAdUntil()), Bankrupt: b.GetBankrupt(), Invested: b.GetInvested(),
		Collected: b.GetCollected(), Resale: b.GetResale(), InDebt: b.GetInDebt(),
		CandidatesUntil: timeOf(b.GetCandidatesUntil()), Seats: int(b.GetSeats()),
		ReviewUntil: timeOf(b.GetReviewUntil()), Tax: int(b.GetTax()), Wages: b.GetWages(), Rent: b.GetRent(),
		Till: b.GetTill(), DebtLimit: b.GetDebtLimit(), Load: b.GetLoad(), Broken: b.GetBroken(),
		BrokenUntil: timeOf(b.GetBrokenUntil()), UpgradeCost: b.GetUpgradeCost(), RepairCost: b.GetRepairCost(),
	}
	for _, s := range b.GetSupplies() {
		out.Supplies = append(out.Supplies, model.BusinessStock{
			Supply: s.GetSupply(), Units: s.GetUnits(), OnWay: s.GetOnWay(), Storage: s.GetStorage(),
			UnitCosts: s.GetUnitCosts(), Use: s.GetUse(),
		})
	}
	for _, o := range b.GetOffers() {
		out.Offers = append(out.Offers, model.BusinessOffer{
			Offer: o.GetOffer(), Price: o.GetPrice(), Demand: o.GetDemand(), Served: o.GetServed(),
			Capacity: o.GetCapacity(), Broken: o.GetBroken(),
		})
	}
	for _, w := range b.GetWorkers() {
		out.Workers = append(out.Workers, fromProtoBusinessWorker(w))
	}
	for _, c := range b.GetCandidates() {
		out.Candidates = append(out.Candidates, model.BusinessCandidate{ID: c.GetId(), Worker: fromProtoBusinessWorker(c.GetWorker())})
	}
	for _, o := range b.GetOrders() {
		out.Orders = append(out.Orders, model.BusinessOrder{
			ID: int(o.GetId()), Supply: o.GetSupply(), Supplier: o.GetSupplier(), Units: o.GetUnits(),
			Arrives: timeOf(o.GetArrives()), FromTill: o.GetFromTill(),
		})
	}
	for _, c := range b.GetContracts() {
		out.Contracts = append(out.Contracts, model.BusinessContract{
			Supply: c.GetSupply(), Supplier: c.GetSupplier(), Below: c.GetBelow(), Units: c.GetUnits(), Stalled: c.GetStalled(),
		})
	}
	if e := b.GetEvent(); e.GetKind() != "" {
		out.Event = &model.BusinessEvent{
			Kind: e.GetKind(), Until: timeOf(e.GetUntil()), Offer: e.GetOffer(), Worker: int(e.GetWorker()),
			Name: e.GetName(), Female: e.GetFemale(), Votes: e.GetVotes(), Units: e.GetUnits(), Pay: e.GetPay(),
			Options: e.GetOptions(),
		}
	}
	for _, n := range b.GetNotes() {
		out.Notes = append(out.Notes, model.BusinessNote{
			At: timeOf(n.GetAt()), Kind: n.GetKind(), Subject: n.GetSubject(), Name: n.GetName(),
			Female: n.GetFemale(), Amount: n.GetAmount(), Money: n.GetMoney(),
		})
	}
	for _, h := range b.GetHours() {
		out.Hours = append(out.Hours, model.BusinessHour{
			Hour: timeOf(h.GetHour()), Sold: h.GetSold(), Income: h.GetIncome(), Wages: h.GetWages(),
			Rent: h.GetRent(), Supplies: h.GetSupplies(), Losses: h.GetLosses(), Spoiled: h.GetSpoiled(),
			Unserved: h.GetUnserved(),
		})
	}

	return out
}

func fromProtoBusinessWorker(w *userv1.BusinessWorker) model.BusinessWorker {
	return model.BusinessWorker{
		ID: int(w.GetId()), Name: w.GetName(), Female: w.GetFemale(), Profession: w.GetProfession(),
		Skill: int(w.GetSkill()), XP: w.GetXp(), NextXP: w.GetNextXp(), Wage: w.GetWage(), Trait: w.GetTrait(),
		Mood: w.GetMood(), Rate: w.GetRate(), Bonus: w.GetBonus(),
	}
}

func fromProtoBusinessSettings(s *userv1.BusinessSettings) model.BusinessSettings {
	out := model.BusinessSettings{Tax: int(s.GetTax()), Wages: int(s.GetWages()), Goods: int(s.GetGoods())}
	for _, p := range s.GetPlans() {
		out.Plans = append(out.Plans, model.BusinessPlan{
			Kind: p.GetKind(), Price: p.GetPrice(), Upgrade: p.GetUpgrade(), Levels: int(p.GetLevels()),
			Demand: p.GetDemand(), Staff: int(p.GetStaff()), Rent: p.GetRent(), Wear: int(p.GetWear()),
			Repair: p.GetRepair(), Till: p.GetTill(), Debt: p.GetDebt(),
			AdPrice: p.GetAdPrice(), AdHours: int(p.GetAdHours()), AdBoost: int(p.GetAdBoost()),
		})
	}

	return out
}

func toProtoBusinessSettings(s model.BusinessSettings) (*userv1.BusinessSettings, error) {
	out := &userv1.BusinessSettings{}
	percents := []struct {
		n    int
		what string
		to   *int32
	}{
		{s.Tax, "the tax", &out.Tax},
		{s.Wages, "the wages", &out.Wages},
		{s.Goods, "the goods", &out.Goods},
	}
	for _, p := range percents {
		n, err := int32Of(p.n, p.what)
		if err != nil {
			return nil, err
		}

		*p.to = n
	}

	for _, p := range s.Plans {
		plan := &userv1.BusinessPlan{
			Kind: p.Kind, Price: p.Price, Upgrade: p.Upgrade, Demand: p.Demand, Rent: p.Rent,
			Repair: p.Repair, Till: p.Till, Debt: p.Debt, AdPrice: p.AdPrice,
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
