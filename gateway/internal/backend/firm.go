package backend

import (
	"context"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Firms is "Бизнес", the economy of the enterprises, which the user
// service keeps with the votes.
type Firms struct {
	api userv1.UserServiceClient
}

func NewFirms(api userv1.UserServiceClient) *Firms {
	return &Firms{api: api}
}

func (b *Firms) Get(ctx context.Context, accessToken string) (model.Firms, error) {
	resp, err := b.api.GetFirms(withToken(ctx, accessToken), &userv1.GetFirmsRequest{})
	if err != nil {
		return model.Firms{}, errs.Wrap("b.api.GetFirms", fromStatus(err))
	}

	return fromProtoFirms(resp), nil
}

// UserFirms are the enterprises a page holds shares of.
func (b *Firms) UserFirms(ctx context.Context, accessToken, nickname string) ([]model.UserFirm, error) {
	resp, err := b.api.GetUserFirms(withToken(ctx, accessToken), &userv1.GetUserFirmsRequest{Nickname: nickname})
	if err != nil {
		return nil, errs.Wrap("b.api.GetUserFirms", fromStatus(err))
	}

	out := make([]model.UserFirm, 0, len(resp.GetFirms()))
	for _, f := range resp.GetFirms() {
		out = append(out, model.UserFirm{
			ID: f.GetId(), Name: f.GetName(), Kind: f.GetKind(), Level: int(f.GetLevel()),
			Shares: f.GetShares(), Total: f.GetTotal(), Director: f.GetDirector(),
		})
	}

	return out, nil
}

func (b *Firms) Run(ctx context.Context, accessToken string, act model.FirmAct) (model.FirmDone, error) {
	workers, err := int32Of(act.Workers, "the workers")
	if err != nil {
		return model.FirmDone{}, err
	}

	resp, err := b.api.RunFirms(withToken(ctx, accessToken), &userv1.RunFirmsRequest{
		Action: act.Action, FirmId: act.FirmID, Kind: act.Kind, Name: act.Name, Good: act.Good, Buy: act.Buy,
		Price: act.Price, Units: act.Units, Workers: workers, Wage: act.Wage, Votes: act.Votes, Shares: act.Shares,
		OrderId: act.OrderID, AutoBuy: act.AutoBuy, BuyMax: act.BuyMax, AutoSell: act.AutoSell, SellMin: act.SellMin,
		Improvement: act.Improvement, ContractId: act.ContractID,
	})
	if err != nil {
		return model.FirmDone{}, errs.Wrap("b.api.RunFirms", fromStatus(err))
	}

	done := model.FirmDone{
		Spent: resp.GetSpent(), Got: resp.GetGot(), Balance: resp.GetBalance(), FirmID: resp.GetFirmId(),
		Firms: fromProtoFirms(resp.GetFirms()),
	}
	for _, d := range resp.GetDeals() {
		done.Deals = append(done.Deals, model.FirmDeal{Good: d.GetGood(), Units: d.GetUnits(), Price: d.GetPrice()})
	}

	return done, nil
}

func (b *Firms) Settings(ctx context.Context, accessToken string) (model.FirmSettings, model.FirmStats, error) {
	resp, err := b.api.GetFirmSettings(withToken(ctx, accessToken), &userv1.GetFirmSettingsRequest{})
	if err != nil {
		return model.FirmSettings{}, model.FirmStats{}, errs.Wrap("b.api.GetFirmSettings", fromStatus(err))
	}

	return fromProtoFirmSettings(resp.GetSettings()),
		model.FirmStats{Firms: resp.GetFirms(), Spent: resp.GetSpent(), Paid: resp.GetPaid(), Fees: resp.GetFees()}, nil
}

func (b *Firms) Set(ctx context.Context, accessToken string, settings model.FirmSettings) error {
	s, err := toProtoFirmSettings(settings)
	if err != nil {
		return err
	}

	_, err = b.api.SetFirmSettings(withToken(ctx, accessToken), &userv1.SetFirmSettingsRequest{Settings: s})

	return errs.Wrap("b.api.SetFirmSettings", fromStatus(err))
}

func timeOf(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

func fromProtoFirms(s *userv1.FirmsState) model.Firms {
	out := model.Firms{Settings: fromProtoFirmSettings(s.GetSettings()), NextHour: timeOf(s.GetNextHour())}
	for _, c := range s.GetCatalog() {
		spec := model.FirmSpec{Kind: c.GetKind(), Tier: int(c.GetTier()), Output: c.GetOutput(), Sells: c.GetSells(), Rate: c.GetRate()}
		for _, in := range c.GetInputs() {
			spec.Inputs = append(spec.Inputs, model.FirmInput{Good: in.GetGood(), Units: in.GetUnits()})
		}
		out.Catalog = append(out.Catalog, spec)
	}
	for _, q := range s.GetQuotes() {
		out.Quotes = append(out.Quotes, model.GoodQuote{
			Good: q.GetGood(), StateBid: q.GetStateBid(), StateAsk: q.GetStateAsk(), CityPrice: q.GetCityPrice(),
			BestBid: q.GetBestBid(), BestAsk: q.GetBestAsk(), Last: q.GetLast(), Volume: q.GetVolume(),
		})
	}
	for _, f := range s.GetFirms() {
		out.Firms = append(out.Firms, fromProtoFirm(f))
	}
	for _, o := range s.GetOrders() {
		out.Orders = append(out.Orders, model.ExchangeOrder{
			ID: o.GetId(), FirmID: o.GetFirmId(), Good: o.GetGood(), Buy: o.GetBuy(), Price: o.GetPrice(),
			Units: o.GetUnits(), PlacedAt: timeOf(o.GetPlacedAt()),
		})
	}
	for _, k := range s.GetContracts() {
		out.Contracts = append(out.Contracts, model.FirmContract{
			ID: k.GetId(), Good: k.GetGood(), Units: k.GetUnits(), Price: k.GetPrice(), Posted: timeOf(k.GetPosted()),
			Deadline: timeOf(k.GetDeadline()), FirmID: k.GetFirmId(), Delivered: k.GetDelivered(),
		})
	}
	out.News = fromProtoNews(s.GetNews())

	return out
}

func fromProtoFirm(f *userv1.Firm) model.Firm {
	out := model.Firm{
		ID: f.GetId(), Kind: f.GetKind(), Name: f.GetName(), DirectorID: f.GetDirectorId(), FoundedAt: timeOf(f.GetFoundedAt()),
		Level: int(f.GetLevel()), Workers: int(f.GetWorkers()), Wage: f.GetWage(), Cash: f.GetCash(), Escrow: f.GetEscrow(),
		Productivity: f.GetProductivity(), Seats: int(f.GetSeats()), UpgradeCost: f.GetUpgradeCost(), Upkeep: f.GetUpkeep(),
		Profit: f.GetProfit(), Shares: f.GetShares(), Worth: f.GetWorth(), Wear: f.GetWear(), RepairCost: f.GetRepairCost(),
		IdleUntil: timeOf(f.GetIdleUntil()), Price: f.GetPrice(), Float: f.GetFloat(),
	}
	for _, i := range f.GetImprovements() {
		out.Improvements = append(out.Improvements, model.FirmImprovement{Kind: i.GetKind(), Rank: int(i.GetRank()), Cost: i.GetCost()})
	}
	for _, s := range f.GetStock() {
		out.Stock = append(out.Stock, model.FirmStock{
			Good: s.GetGood(), Units: s.GetUnits(), Capacity: s.GetCapacity(), Incoming: s.GetIncoming(),
			AutoBuy: s.GetAutoBuy(), BuyMax: s.GetBuyMax(), AutoSell: s.GetAutoSell(), SellMin: s.GetSellMin(),
		})
	}
	for _, h := range f.GetHours() {
		out.Hours = append(out.Hours, model.FirmHour{
			Hour: timeOf(h.GetHour()), Made: h.GetMade(), Bought: h.GetBought(), Sold: h.GetSold(), Income: h.GetIncome(),
			Purchases: h.GetPurchases(), Wages: h.GetWages(), Upkeep: h.GetUpkeep(), Fees: h.GetFees(),
		})
	}
	for _, h := range f.GetHolders() {
		out.Holders = append(out.Holders, model.FirmHolder{UserID: h.GetUserId(), Shares: h.GetShares()})
	}

	return out
}

func fromProtoNews(news []*userv1.FirmEvent) []model.FirmEvent {
	out := make([]model.FirmEvent, 0, len(news))
	for _, e := range news {
		out = append(out, model.FirmEvent{
			ID: e.GetId(), Kind: e.GetKind(), FirmID: e.GetFirmId(), Kinds: e.GetKinds(),
			Start: timeOf(e.GetStart()), End: timeOf(e.GetEnd()),
		})
	}

	return out
}

func fromProtoFirmSettings(s *userv1.FirmSettings) model.FirmSettings {
	out := model.FirmSettings{
		Wage: s.GetWage(), Fee: int(s.GetFee()), Floor: int(s.GetFloor()), Ceiling: int(s.GetCeiling()),
		Retail: int(s.GetRetail()), Storage: int(s.GetStorage()), Capital: int(s.GetCapital()),
		MaxFirms: int(s.GetMaxFirms()), FoundingShares: s.GetFoundingShares(), FloatShares: s.GetFloatShares(), MaxShares: s.GetMaxShares(),
	}
	for _, p := range s.GetPlans() {
		out.Plans = append(out.Plans, model.FirmPlan{
			Kind: p.GetKind(), Price: p.GetPrice(), Upgrade: p.GetUpgrade(), Levels: int(p.GetLevels()),
			Staff: int(p.GetStaff()), Upkeep: p.GetUpkeep(),
		})
	}
	for _, g := range s.GetGoods() {
		out.Goods = append(out.Goods, model.GoodPlan{Good: g.GetGood(), Base: g.GetBase(), Norm: g.GetNorm()})
	}

	return out
}

func toProtoFirmSettings(s model.FirmSettings) (*userv1.FirmSettings, error) {
	out := &userv1.FirmSettings{Wage: s.Wage, FoundingShares: s.FoundingShares, FloatShares: s.FloatShares, MaxShares: s.MaxShares}
	ints := []struct {
		n    int
		what string
		to   *int32
	}{
		{s.Fee, "the fee", &out.Fee},
		{s.Floor, "the price the state pays", &out.Floor},
		{s.Ceiling, "the price the state asks", &out.Ceiling},
		{s.Retail, "the price the city pays", &out.Retail},
		{s.Storage, "the storage", &out.Storage},
		{s.Capital, "the capital", &out.Capital},
		{s.MaxFirms, "the enterprises of a page", &out.MaxFirms},
	}
	for _, i := range ints {
		n, err := int32Of(i.n, i.what)
		if err != nil {
			return nil, err
		}

		*i.to = n
	}

	for _, p := range s.Plans {
		levels, err := int32Of(p.Levels, "the levels")
		if err != nil {
			return nil, err
		}

		staff, err := int32Of(p.Staff, "the staff")
		if err != nil {
			return nil, err
		}

		out.Plans = append(out.Plans, &userv1.FirmPlan{
			Kind: p.Kind, Price: p.Price, Upgrade: p.Upgrade, Levels: levels, Staff: staff, Upkeep: p.Upkeep,
		})
	}
	for _, g := range s.Goods {
		out.Goods = append(out.Goods, &userv1.GoodPlan{Good: g.Good, Base: g.Base, Norm: g.Norm})
	}

	return out, nil
}
