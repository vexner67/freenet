package grpc

import (
	"context"
	"math"
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

type BusinessService interface {
	Businesses(ctx context.Context, userID uuid.UUID, now time.Time) (service.BusinessesView, error)
	UserBusinesses(ctx context.Context, nickname user.Nickname, now time.Time) ([]user.Business, error)
	Buy(ctx context.Context, userID uuid.UUID, kind user.BusinessKind, now time.Time) (service.BusinessDone, error)
	Act(ctx context.Context, userID uuid.UUID, kind user.BusinessKind, act service.BusinessAct, now time.Time) (service.BusinessDone, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.BusinessSettings, error)
	SetRules(ctx context.Context, adminID uuid.UUID, rules user.BusinessRules) error
}

func toProtoBusinessSettings(r user.BusinessRules) *userv1.BusinessSettings {
	plans := make([]*userv1.BusinessPlan, 0, len(r.Plans()))
	for _, p := range r.Plans() {
		plans = append(plans, &userv1.BusinessPlan{
			Kind:    p.Kind.String(),
			Price:   p.Price,
			Upgrade: p.Upgrade,
			Levels:  int32(p.Levels), //nolint:gosec // at most user.MaxBusinessLevels
			Demand:  int64(p.Demand),
			Staff:   int32(p.Staff), //nolint:gosec // at most user.MaxBusinessStaff
			Rent:    p.Rent,
			Wear:    int32(p.Wear), //nolint:gosec // at most user.MaxBusinessWear
			Repair:  p.Repair,
			Till:    p.Till,
			Debt:    p.Debt,
			AdPrice: p.AdPrice,
			AdHours: int32(p.AdHours), //nolint:gosec // at most user.MaxBusinessAdHours
			AdBoost: int32(p.AdBoost), //nolint:gosec // at most user.MaxBusinessAdBoost
		})
	}

	return &userv1.BusinessSettings{
		Plans: plans,
		Tax:   int32(r.Tax()),   //nolint:gosec // at most user.MaxBusinessTax
		Wages: int32(r.Wages()), //nolint:gosec // at most user.MaxBusinessPercent
		Goods: int32(r.Goods()), //nolint:gosec // at most user.MaxBusinessPercent
	}
}

func fromProtoBusinessSettings(s *userv1.BusinessSettings) (user.BusinessRules, error) {
	plans := make([]user.BusinessPlan, 0, len(s.GetPlans()))
	for _, p := range s.GetPlans() {
		demand := p.GetDemand()
		if demand < 0 || demand > user.MaxBusinessPercent {
			return user.BusinessRules{}, user.ErrBusinessPercent
		}

		plans = append(plans, user.BusinessPlan{
			Kind:    user.BusinessKind(p.GetKind()),
			Price:   p.GetPrice(),
			Upgrade: p.GetUpgrade(),
			Levels:  int(p.GetLevels()),
			Demand:  int(demand),
			Staff:   int(p.GetStaff()),
			Rent:    p.GetRent(),
			Wear:    int(p.GetWear()),
			Repair:  p.GetRepair(),
			Till:    p.GetTill(),
			Debt:    p.GetDebt(),
			AdPrice: p.GetAdPrice(),
			AdHours: int(p.GetAdHours()),
			AdBoost: int(p.GetAdBoost()),
		})
	}

	return user.NewBusinessRules(plans, int(s.GetTax()), int(s.GetWages()), int(s.GetGoods()))
}

// timeAfter is t when it is after now, nil otherwise: what has ended is
// not told.
func timeAfter(t, now time.Time) *timestamppb.Timestamp {
	if !t.After(now) {
		return nil
	}

	return timestamppb.New(t)
}

func toProtoBusinessCatalog() []*userv1.BusinessCatalog {
	catalog := make([]*userv1.BusinessCatalog, 0, len(user.BusinessKinds))
	for _, kind := range user.BusinessKinds {
		spec, err := user.BusinessCatalog(kind)
		if err != nil {
			continue
		}

		c := &userv1.BusinessCatalog{Kind: kind.String()}
		for _, s := range spec.Supplies {
			c.Supplies = append(c.Supplies, &userv1.BusinessSupplySpec{
				Supply: s.Supply.String(), Cost: s.Cost, Spoil: int32(s.Spoil), Storage: s.Storage, //nolint:gosec // a catalog constant
			})
		}
		for _, o := range spec.Offers {
			offer := &userv1.BusinessOfferSpec{Offer: o.Offer.String(), Profession: o.Profession.String(), Fair: o.Fair, Demand: o.Demand}
			for _, in := range o.Recipe {
				offer.Recipe = append(offer.Recipe, &userv1.BusinessIngredient{Supply: in.Supply.String(), Units: in.Units})
			}
			c.Offers = append(c.Offers, offer)
		}
		for _, p := range spec.Professions {
			c.Professions = append(c.Professions, &userv1.BusinessProfessionSpec{Profession: p.Profession.String(), Wage: p.Wage, Serve: p.Serve})
		}

		catalog = append(catalog, c)
	}

	return catalog
}

func toProtoBusinessSuppliers() []*userv1.BusinessSupplier {
	specs := user.SupplierSpecs()
	suppliers := make([]*userv1.BusinessSupplier, 0, len(specs))
	for _, s := range specs {
		suppliers = append(suppliers, &userv1.BusinessSupplier{
			Supplier: s.Supplier.String(), Percent: s.Percent, Hours: s.Hours, Short: int32(s.Short), //nolint:gosec // a catalog constant
		})
	}

	return suppliers
}

func toProtoBusinessWorker(w user.Worker) *userv1.BusinessWorker {
	return &userv1.BusinessWorker{
		Id:         int32(w.ID), //nolint:gosec // a counter of a business
		Name:       w.Name,
		Female:     w.Female,
		Profession: w.Profession.String(),
		Skill:      int32(w.Skill), //nolint:gosec // at most user.MaxSkill
		Xp:         w.XP,
		NextXp:     w.NextSkillHours(),
		Wage:       w.Wage,
		Trait:      string(w.Trait),
		Mood:       w.Mood,
		Rate:       w.Rate(),
		Bonus:      w.BonusCost(),
	}
}

func toProtoBusinessHour(h user.BusinessHour) *userv1.BusinessHour {
	sold := make(map[string]float64, len(h.Sold))
	for o, units := range h.Sold {
		sold[o.String()] = units
	}

	return &userv1.BusinessHour{
		Hour: timestamppb.New(h.Hour), Sold: sold, Income: h.Income, Wages: h.Wages, Rent: h.Rent,
		Supplies: h.Supplies, Losses: h.Losses, Spoiled: h.Spoiled, Unserved: h.Unserved,
	}
}

func toProtoBusinessEvent(b user.Business) *userv1.BusinessEvent {
	e, ok := b.Event()
	if !ok {
		return nil
	}

	return &userv1.BusinessEvent{
		Kind: string(e.Kind), Until: timestamppb.New(e.Until), Offer: e.Offer.String(),
		Worker: int32(e.Worker), Name: e.Name, Female: e.Female, //nolint:gosec // a counter of a business
		Votes: e.Votes, Units: e.Units, Pay: e.Pay, Options: e.Options(),
	}
}

func toProtoBusinessStock(b user.Business, spec user.KindSpec, r user.BusinessRules, rates user.BusinessRates) []*userv1.BusinessStock {
	stock := make([]*userv1.BusinessStock, 0, len(spec.Supplies))
	for _, s := range spec.Supplies {
		costs := map[string]float64{}
		for _, sup := range user.SupplierSpecs() {
			costs[sup.Supplier.String()] = b.UnitCost(s, sup, r)
		}

		stock = append(stock, &userv1.BusinessStock{
			Supply: s.Supply.String(), Units: b.Stock(s.Supply), OnWay: b.OnTheWay(s.Supply),
			Storage: b.StorageOf(s), UnitCosts: costs, Use: rates.Use[s.Supply],
		})
	}

	return stock
}

func toProtoBusiness(b user.Business, plan user.BusinessPlan, r user.BusinessRules, now time.Time) *userv1.Business {
	spec, _ := user.BusinessCatalog(b.Kind()) // a kept business is of a known kind
	rates := b.Rates(plan, r, now)
	resale, _ := b.Resale() // 0 in debt: the business does not sell then

	pb := &userv1.Business{
		Kind:            b.Kind().String(),
		Level:           int32(b.Level()),     //nolint:gosec // at most user.MaxBusinessLevels
		Condition:       int32(b.Condition()), //nolint:gosec // at most user.FullCondition
		Cash:            b.Cash(),
		AdUntil:         timeAfter(b.AdUntil(), now),
		Bankrupt:        b.Bankrupt(),
		Invested:        b.Invested(),
		Collected:       b.CollectedVotes(),
		Resale:          resale,
		InDebt:          b.InDebt(),
		Supplies:        toProtoBusinessStock(b, spec, r, rates),
		CandidatesUntil: timestamppb.New(user.CandidatesRenewAt(now)),
		Event:           toProtoBusinessEvent(b),
		Seats:           int32(b.Seats(plan)), //nolint:gosec // at most user.MaxBusinessStaff × levels
		ReviewUntil:     timeAfter(b.ReviewUntil(), now),
		Tax:             int32(b.TaxOf(r)), //nolint:gosec // at most user.MaxBusinessTax
		Wages:           rates.Wages,
		Rent:            rates.Rent,
		Till:            int64(b.TillAt(plan)),
		DebtLimit:       int64(b.DebtAt(plan)),
		Load:            map[string]float64{},
		RepairCost:      b.RepairCost(plan),
	}
	if b.Level() < plan.Levels {
		pb.UpgradeCost = b.UpgradeCost(plan)
	}

	for _, o := range rates.Offers {
		pb.Offers = append(pb.Offers, &userv1.BusinessOfferState{
			Offer: o.Offer.String(), Price: b.Price(o.Offer), Demand: o.Demand, Served: o.Served,
			Capacity: o.Capacity, Broken: o.Broken,
		})
	}
	for prof, load := range rates.Load {
		if !math.IsInf(load, 0) {
			pb.Load[prof.String()] = load
		}
	}
	for _, w := range b.Workers() {
		pb.Workers = append(pb.Workers, toProtoBusinessWorker(w))
	}
	if !b.Bankrupt() {
		for _, c := range b.Candidates(r, now) {
			pb.Candidates = append(pb.Candidates, &userv1.BusinessCandidate{Id: c.ID, Worker: toProtoBusinessWorker(c.Worker)})
		}
	}
	for _, o := range b.Orders() {
		pb.Orders = append(pb.Orders, &userv1.BusinessOrder{
			Id: int32(o.ID), Supply: o.Supply.String(), Supplier: o.Supplier.String(), //nolint:gosec // a counter of a business
			Units: o.Units, Arrives: timestamppb.New(o.Arrives), FromTill: o.FromTill,
		})
	}
	for _, c := range b.Contracts() {
		pb.Contracts = append(pb.Contracts, &userv1.BusinessContract{
			Supply: c.Supply.String(), Supplier: c.Supplier.String(), Below: c.Below, Units: c.Units, Stalled: c.Stalled,
		})
	}
	notes := b.Notes()
	for i := len(notes) - 1; i >= 0; i-- {
		n := notes[i]
		pb.Notes = append(pb.Notes, &userv1.BusinessNote{
			At: timestamppb.New(n.At), Kind: string(n.Kind), Subject: n.Subject, Name: n.Name,
			Female: n.Female, Amount: n.Amount, Money: n.Money,
		})
	}
	for _, h := range b.Hours() {
		pb.Hours = append(pb.Hours, toProtoBusinessHour(h))
	}
	if broken, until := b.Broken(); broken != "" {
		pb.Broken = broken.String()
		if !until.IsZero() {
			pb.BrokenUntil = timestamppb.New(until)
		}
	}

	return pb
}

func toProtoBusinesses(v service.BusinessesView) *userv1.BusinessesState {
	businesses := make([]*userv1.Business, 0, len(v.Businesses))
	for _, b := range v.Businesses {
		plan, err := v.Rules.Plan(b.Kind())
		if err != nil {
			continue
		}

		businesses = append(businesses, toProtoBusiness(b, plan, v.Rules, v.At))
	}

	return &userv1.BusinessesState{
		Settings:   toProtoBusinessSettings(v.Rules),
		Businesses: businesses,
		Catalog:    toProtoBusinessCatalog(),
		Suppliers:  toProtoBusinessSuppliers(),
	}
}

func (h *UserHandler) GetBusinesses(ctx context.Context, _ *userv1.GetBusinessesRequest) (*userv1.BusinessesState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	v, err := h.businesses.Businesses(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	return toProtoBusinesses(v), nil
}

func (h *UserHandler) GetUserBusinesses(ctx context.Context, req *userv1.GetUserBusinessesRequest) (*userv1.GetUserBusinessesResponse, error) {
	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	owned, err := h.businesses.UserBusinesses(ctx, nickname, time.Now())
	if err != nil {
		return nil, err
	}

	out := make([]*userv1.UserBusiness, len(owned))
	for i, b := range owned {
		out[i] = &userv1.UserBusiness{
			Kind:     b.Kind().String(),
			Level:    int32(b.Level()),
			Workers:  int32(len(b.Workers())),
			Bankrupt: b.Bankrupt(),
		}
	}

	return &userv1.GetUserBusinessesResponse{Businesses: out}, nil
}

// done answers an action with the businesses as they are after it.
func (h *UserHandler) done(ctx context.Context, userID uuid.UUID, d service.BusinessDone) (*userv1.RunBusinessResponse, error) {
	v, err := h.businesses.Businesses(ctx, userID, d.At)
	if err != nil {
		return nil, err
	}

	return &userv1.RunBusinessResponse{
		Spent:      d.Spent,
		Got:        d.Got,
		Tax:        d.Tax,
		Balance:    d.Balance,
		Businesses: toProtoBusinesses(v),
	}, nil
}

func (h *UserHandler) BuyBusiness(ctx context.Context, req *userv1.BuyBusinessRequest) (*userv1.RunBusinessResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewBusinessKind(req.GetKind())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	d, err := h.businesses.Buy(ctx, userID, kind, time.Now())
	if err != nil {
		return nil, err
	}

	return h.done(ctx, userID, d)
}

func (h *UserHandler) RunBusiness(ctx context.Context, req *userv1.RunBusinessRequest) (*userv1.RunBusinessResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewBusinessKind(req.GetKind())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	action, err := user.NewBusinessAction(req.GetAction())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	d, err := h.businesses.Act(ctx, userID, kind, service.BusinessAct{
		Action:    action,
		Amount:    req.GetAmount(),
		Price:     req.GetPrice(),
		Offer:     user.Offer(req.GetOffer()),
		Supply:    user.Supply(req.GetSupply()),
		Supplier:  user.Supplier(req.GetSupplier()),
		Below:     req.GetBelow(),
		Worker:    int(req.GetWorker()),
		Candidate: req.GetCandidate(),
		Option:    req.GetOption(),
	}, time.Now())
	if err != nil {
		return nil, err
	}

	return h.done(ctx, userID, d)
}

func (h *UserHandler) GetBusinessSettings(ctx context.Context, _ *userv1.GetBusinessSettingsRequest) (*userv1.GetBusinessSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.businesses.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetBusinessSettingsResponse{
		Settings:   toProtoBusinessSettings(s.Rules),
		Businesses: s.Stats.Businesses,
		Spent:      s.Stats.Spent,
		Paid:       s.Stats.Paid,
	}, nil
}

func (h *UserHandler) SetBusinessSettings(ctx context.Context, req *userv1.SetBusinessSettingsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	rules, err := fromProtoBusinessSettings(req.GetSettings())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.businesses.SetRules(ctx, adminID, rules); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
