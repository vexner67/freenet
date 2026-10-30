package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FirmService interface {
	View(ctx context.Context, now time.Time) (service.FirmsView, error)
	UserFirms(ctx context.Context, nickname user.Nickname, now time.Time) ([]service.UserFirm, error)
	Act(ctx context.Context, userID uuid.UUID, act service.FirmAct, now time.Time) (service.FirmDone, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.FirmSettings, error)
	SetRules(ctx context.Context, adminID uuid.UUID, rules user.FirmRules) error
}

func toProtoFirmSettings(r user.FirmRules) *userv1.FirmSettings {
	s := &userv1.FirmSettings{
		Wage:           r.Wage,
		Fee:            int32(r.Fee),      //nolint:gosec // at most user.MaxFirmFee
		Floor:          int32(r.Floor),    //nolint:gosec // at most user.MaxFirmPercent
		Ceiling:        int32(r.Ceiling),  //nolint:gosec // at most user.MaxFirmPercent
		Retail:         int32(r.Retail),   //nolint:gosec // at most user.MaxFirmPercent
		Storage:        int32(r.Storage),  //nolint:gosec // at most user.MaxFirmStorage
		Capital:        int32(r.Capital),  //nolint:gosec // at most 100
		MaxFirms:       int32(r.MaxFirms), //nolint:gosec // at most user.MaxFirmsDirected
		FoundingShares: r.Founding,
		FloatShares:    r.Float,
		MaxShares:      r.MaxShares,
	}
	for _, p := range r.Plans {
		s.Plans = append(s.Plans, &userv1.FirmPlan{
			Kind: p.Kind.String(), Price: p.Price, Upgrade: p.Upgrade,
			Levels: int32(p.Levels), Staff: int32(p.Staff), Upkeep: p.Upkeep, //nolint:gosec // at most user.MaxFirmLevels and user.MaxFirmStaff
		})
	}
	for _, g := range r.Goods {
		s.Goods = append(s.Goods, &userv1.GoodPlan{Good: g.Good.String(), Base: g.Base, Norm: g.Norm})
	}

	return s
}

func fromProtoFirmSettings(s *userv1.FirmSettings) (user.FirmRules, error) {
	r := user.FirmRules{
		Wage: s.GetWage(), Fee: int(s.GetFee()), Floor: int(s.GetFloor()), Ceiling: int(s.GetCeiling()),
		Retail: int(s.GetRetail()), Storage: int(s.GetStorage()), Capital: int(s.GetCapital()),
		MaxFirms: int(s.GetMaxFirms()), Founding: s.GetFoundingShares(), Float: s.GetFloatShares(), MaxShares: s.GetMaxShares(),
	}
	for _, p := range s.GetPlans() {
		r.Plans = append(r.Plans, user.FirmPlan{
			Kind: user.FirmKind(p.GetKind()), Price: p.GetPrice(), Upgrade: p.GetUpgrade(),
			Levels: int(p.GetLevels()), Staff: int(p.GetStaff()), Upkeep: p.GetUpkeep(),
		})
	}
	for _, g := range s.GetGoods() {
		r.Goods = append(r.Goods, user.GoodPlan{Good: user.Good(g.GetGood()), Base: g.GetBase(), Norm: g.GetNorm()})
	}

	return user.NewFirmRules(r)
}

func toProtoFirmCatalog() []*userv1.FirmSpec {
	specs := user.FirmSpecs()
	out := make([]*userv1.FirmSpec, 0, len(specs))
	for _, s := range specs {
		spec := &userv1.FirmSpec{Kind: s.Kind.String(), Tier: int32(s.Tier), Output: s.Output.String(), Rate: map[string]float64{}} //nolint:gosec // a catalog constant
		for _, in := range s.Inputs {
			spec.Inputs = append(spec.Inputs, &userv1.FirmInput{Good: in.Good.String(), Units: in.Units})
		}
		for _, g := range s.Sells {
			spec.Sells = append(spec.Sells, g.String())
		}
		for g, rate := range s.Rate {
			spec.Rate[g.String()] = rate
		}

		out = append(out, spec)
	}

	return out
}

func toProtoFirmHour(h user.FirmHour) *userv1.FirmHour {
	return &userv1.FirmHour{
		Hour: timestamppb.New(h.Hour), Made: h.Made, Bought: h.Bought, Sold: h.Sold, Income: h.Income,
		Purchases: h.Purchases, Wages: h.Wages, Upkeep: h.Upkeep, Fees: h.Fees, Hired: h.Hired, Left: h.Left,
	}
}

func toProtoFirm(v service.FirmsView, f *user.Firm) *userv1.Firm {
	plan, _ := v.Rules.Plan(f.Kind()) // a kept enterprise is of a known kind
	spec, _ := user.FirmCatalog(f.Kind())
	c := v.City

	pb := &userv1.Firm{
		Id: f.ID().String(), Kind: f.Kind().String(), Name: f.Name().String(), DirectorId: f.DirectorID().String(),
		FoundedAt: timestamppb.New(f.FoundedAt()), Level: int32(f.Level()), Workers: int32(f.Workers()), //nolint:gosec // at most user.MaxFirmLevels × user.MaxFirmStaff
		Wage: f.Wage(), Cash: f.Cash(), Escrow: c.Escrow(f.ID()), Productivity: v.Rules.Productivity(f.Wage()),
		Seats: int32(plan.Staff * f.Level()), Upkeep: plan.Upkeep * int64(f.Level()), Profit: f.Profit(), //nolint:gosec // as above
		Shares: c.Shares(f.ID()), Worth: c.Worth(v.Rules, f), Wear: f.Wear(), RepairCost: user.RepairCost(plan, f),
		Price: f.Price(), Float: f.Float(), Wanted: int32(f.Wanted()), //nolint:gosec // at most the seats
	}
	if f.Level() < plan.Levels {
		pb.UpgradeCost = plan.UpgradeCost(f.Level())
	}
	if f.IdleUntil().After(v.At) {
		pb.IdleUntil = timestamppb.New(f.IdleUntil())
	}
	for _, imp := range user.Improvements {
		up := &userv1.FirmImprovement{Kind: string(imp), Rank: int32(f.Rank(imp))} //nolint:gosec // at most user.MaxRank
		if f.Rank(imp) < user.MaxRank {
			up.Cost = user.ImprovementCost(plan, f.Rank(imp))
		}
		pb.Improvements = append(pb.Improvements, up)
	}

	for _, g := range spec.Uses() {
		a := f.Auto(g)
		pb.Stock = append(pb.Stock, &userv1.FirmStock{
			Good: g.String(), Units: f.Stock(g), Capacity: v.Rules.Capacity(spec, plan, f.Level(), g),
			Incoming: c.Incoming(f.ID(), g), AutoBuy: a.Buy, BuyMax: a.BuyMax, AutoSell: a.Sell, SellMin: a.SellMin,
		})
	}
	for _, h := range f.Hours() {
		pb.Hours = append(pb.Hours, toProtoFirmHour(h))
	}
	for _, r := range f.Routes() {
		pb.Routes = append(pb.Routes, &userv1.FirmRoute{Id: r.ID.String(), ToFirmId: r.To.String(), Good: r.Good.String(), From: r.From, Units: r.Units, Auto: r.Auto})
	}
	if forecast, ok := v.Forecast[f.ID()]; ok {
		pb.Forecast, pb.ForecastCash = toProtoFirmHour(forecast.Hour), forecast.Cash
	}
	for _, h := range c.Holdings(f.ID()) {
		pb.Holders = append(pb.Holders, &userv1.FirmHolder{UserId: h.UserID.String(), Shares: h.Shares})
	}

	return pb
}

func toProtoFirms(v service.FirmsView) *userv1.FirmsState {
	c := v.City
	state := &userv1.FirmsState{
		Settings: toProtoFirmSettings(v.Rules),
		Catalog:  toProtoFirmCatalog(),
		NextHour: timestamppb.New(c.Hour().Add(time.Hour)),
	}

	for _, q := range c.Quotes(v.Rules, v.At) {
		state.Quotes = append(state.Quotes, &userv1.GoodQuote{
			Good: q.Good.String(), StateBid: q.StateBid, StateAsk: q.StateAsk, CityPrice: q.CityPrice,
			BestBid: q.BestBid, BestAsk: q.BestAsk, Last: q.Last, Volume: q.Volume,
		})
	}
	for _, f := range c.Firms() {
		state.Firms = append(state.Firms, toProtoFirm(v, f))
	}
	for _, o := range c.Orders() {
		state.Orders = append(state.Orders, &userv1.ExchangeOrder{
			Id: o.ID.String(), FirmId: o.FirmID.String(), Good: o.Good.String(), Buy: o.Buy,
			Price: o.Price, Units: o.Units, PlacedAt: timestamppb.New(o.PlacedAt),
		})
	}
	for _, k := range c.Contracts() {
		contract := &userv1.FirmContract{
			Id: k.ID.String(), Good: k.Good.String(), Units: k.Units, Price: k.Price, Posted: timestamppb.New(k.Posted),
			Deadline: timestamppb.New(k.Deadline), Delivered: k.Delivered,
		}
		if k.IsTaken() {
			contract.FirmId = k.FirmID.String()
		}
		state.Contracts = append(state.Contracts, contract)
	}
	state.News = toProtoNews(c.News())
	state.Tenders = toProtoTenders(c.Tenders())

	l := c.Labour(v.Rules)
	state.Labour = &userv1.FirmLabour{
		People: int32(l.People), Employed: int32(l.Employed), Jobs: int32(l.Jobs), Wage: l.Wage, //nolint:gosec // at most the seats of the city
	}

	return state
}

func toProtoTenders(tenders []user.Tender) []*userv1.StockTender {
	out := make([]*userv1.StockTender, len(tenders))
	for i, t := range tenders {
		out[i] = &userv1.StockTender{
			Id: t.ID.String(), FirmId: t.FirmID.String(), UserId: t.UserID.String(), Price: t.Price, Shares: t.Shares,
			PlacedAt: timestamppb.New(t.PlacedAt), Until: timestamppb.New(t.Until),
		}
	}

	return out
}

func toProtoNews(news []user.Event) []*userv1.FirmEvent {
	out := make([]*userv1.FirmEvent, 0, len(news))
	for _, e := range news {
		pb := &userv1.FirmEvent{Id: e.ID.String(), Kind: string(e.Kind), Start: timestamppb.New(e.Start), End: timestamppb.New(e.End)}
		if e.FirmID != uuid.Nil {
			pb.FirmId = e.FirmID.String()
		}
		for _, k := range e.Kinds() {
			pb.Kinds = append(pb.Kinds, k.String())
		}
		out = append(out, pb)
	}

	return out
}

func (h *UserHandler) GetFirms(ctx context.Context, _ *userv1.GetFirmsRequest) (*userv1.FirmsState, error) {
	if _, err := currentUserID(ctx); err != nil {
		return nil, err
	}

	v, err := h.firms.View(ctx, time.Now())
	if err != nil {
		return nil, err
	}

	return toProtoFirms(v), nil
}

// optionalID is the ID of s, or none for an empty s.
func optionalID(s, what string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid "+what)
	}

	return id, nil
}

// fromProtoFirmAct is the action of req: the values it names are checked
// here, the rest by the domain.
func fromProtoFirmAct(req *userv1.RunFirmsRequest) (service.FirmAct, error) {
	action, err := user.NewFirmAction(req.GetAction())
	if err != nil {
		return service.FirmAct{}, status.Error(codes.InvalidArgument, err.Error())
	}

	act := service.FirmAct{
		Action: action, Workers: int(req.GetWorkers()), Wage: req.GetWage(), Votes: req.GetVotes(), Shares: req.GetShares(),
		Order: user.ExchangeOrder{Buy: req.GetBuy(), Price: req.GetPrice(), Units: req.GetUnits()},
		Auto:  user.Auto{Buy: req.GetAutoBuy(), BuyMax: req.GetBuyMax(), Sell: req.GetAutoSell(), SellMin: req.GetSellMin()},
		Route: user.SupplyRoute{From: req.GetFrom(), Units: req.GetUnits()},
	}

	ids := []struct {
		to   *uuid.UUID
		from string
		what string
	}{
		{&act.FirmID, req.GetFirmId(), "enterprise"}, {&act.OrderID, req.GetOrderId(), "order"}, {&act.ContractID, req.GetContractId(), "contract"},
		{&act.Route.To, req.GetToFirmId(), "enterprise"}, {&act.RouteID, req.GetRouteId(), "supply"},
	}
	for _, id := range ids {
		if *id.to, err = optionalID(id.from, id.what); err != nil {
			return service.FirmAct{}, err
		}
	}

	if req.GetKind() != "" || action == user.FirmFound {
		if act.Kind, err = user.NewFirmKind(req.GetKind()); err != nil {
			return service.FirmAct{}, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	if req.GetName() != "" || action == user.FirmFound || action == user.FirmRename {
		if act.Name, err = user.NewFirmName(req.GetName()); err != nil {
			return service.FirmAct{}, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	if req.GetImprovement() != "" || action == user.FirmImprove {
		if act.Improvement, err = user.NewImprovement(req.GetImprovement()); err != nil {
			return service.FirmAct{}, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	if req.GetGood() != "" {
		if act.Order.Good, err = user.NewGood(req.GetGood()); err != nil {
			return service.FirmAct{}, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	return act, nil
}

func (h *UserHandler) RunFirms(ctx context.Context, req *userv1.RunFirmsRequest) (*userv1.RunFirmsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	act, err := fromProtoFirmAct(req)
	if err != nil {
		return nil, err
	}

	d, err := h.firms.Act(ctx, userID, act, time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.RunFirmsResponse{Spent: d.Spent, Got: d.Got, Balance: d.Balance, Firms: toProtoFirms(d.View)}
	if d.FirmID != uuid.Nil {
		resp.FirmId = d.FirmID.String()
	}
	for _, deal := range d.Deals {
		resp.Deals = append(resp.Deals, &userv1.FirmDeal{Good: deal.Good.String(), Units: deal.Units, Price: deal.Price})
	}

	return resp, nil
}

func (h *UserHandler) GetUserFirms(ctx context.Context, req *userv1.GetUserFirmsRequest) (*userv1.GetUserFirmsResponse, error) {
	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	held, err := h.firms.UserFirms(ctx, nickname, time.Now())
	if err != nil {
		return nil, err
	}

	out := make([]*userv1.UserFirm, len(held))
	for i, uf := range held {
		out[i] = &userv1.UserFirm{
			Id: uf.Firm.ID().String(), Name: uf.Firm.Name().String(), Kind: uf.Firm.Kind().String(),
			Level: int32(uf.Firm.Level()), Shares: uf.Shares, Total: uf.Total, Director: uf.Director, //nolint:gosec // at most user.MaxFirmLevels
		}
	}

	return &userv1.GetUserFirmsResponse{Firms: out}, nil
}

func (h *UserHandler) GetFirmSettings(ctx context.Context, _ *userv1.GetFirmSettingsRequest) (*userv1.GetFirmSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.firms.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetFirmSettingsResponse{
		Settings: toProtoFirmSettings(s.Rules),
		Firms:    int64(s.Firms),
		Spent:    s.Stats.Spent,
		Paid:     s.Stats.Paid,
		Fees:     s.Stats.Fees,
	}, nil
}

func (h *UserHandler) SetFirmSettings(ctx context.Context, req *userv1.SetFirmSettingsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	rules, err := fromProtoFirmSettings(req.GetSettings())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.firms.SetRules(ctx, adminID, rules); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
