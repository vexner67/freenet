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

type BusinessService interface {
	Businesses(ctx context.Context, userID uuid.UUID, now time.Time) (service.BusinessesView, error)
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
			Demand:  p.Demand,
			Fair:    p.Fair,
			Cost:    p.Cost,
			Storage: p.Storage,
			Serve:   p.Serve,
			Staff:   int32(p.Staff), //nolint:gosec // at most user.MaxBusinessStaff
			Wage:    p.Wage,
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
		Tax:   int32(r.Tax()), //nolint:gosec // at most user.MaxBusinessTax
	}
}

func fromProtoBusinessSettings(s *userv1.BusinessSettings) (user.BusinessRules, error) {
	plans := make([]user.BusinessPlan, 0, len(s.GetPlans()))
	for _, p := range s.GetPlans() {
		plans = append(plans, user.BusinessPlan{
			Kind:    user.BusinessKind(p.GetKind()),
			Price:   p.GetPrice(),
			Upgrade: p.GetUpgrade(),
			Levels:  int(p.GetLevels()),
			Demand:  p.GetDemand(),
			Fair:    p.GetFair(),
			Cost:    p.GetCost(),
			Storage: p.GetStorage(),
			Serve:   p.GetServe(),
			Staff:   int(p.GetStaff()),
			Wage:    p.GetWage(),
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

	return user.NewBusinessRules(plans, int(s.GetTax()))
}

func toProtoBusiness(b user.Business, plan user.BusinessPlan, now time.Time) *userv1.Business {
	s := b.State()
	h := b.Hour(plan, now)
	resale, _ := b.Resale() // 0 in debt: the business does not sell then

	var adUntil *timestamppb.Timestamp
	if s.AdUntil.After(now) {
		adUntil = timestamppb.New(s.AdUntil)
	}

	return &userv1.Business{
		Kind:      s.Kind.String(),
		Level:     int32(s.Level), //nolint:gosec // at most user.MaxBusinessLevels
		Stock:     s.Stock,
		Price:     s.Price,
		Staff:     int32(s.Staff),     //nolint:gosec // at most user.MaxBusinessStaff
		Condition: int32(s.Condition), //nolint:gosec // at most user.FullCondition
		Cash:      s.Cash,
		AdUntil:   adUntil,
		Bankrupt:  s.Bankrupt,
		Invested:  s.Invested,
		Collected: s.Collected,
		Customers: h.Customers,
		Served:    h.Served,
		Income:    h.Income,
		Costs:     h.Costs,
		Resale:    resale,
		InDebt:    s.Cash < 0,
	}
}

func toProtoBusinesses(v service.BusinessesView) *userv1.BusinessesState {
	businesses := make([]*userv1.Business, 0, len(v.Businesses))
	for _, b := range v.Businesses {
		plan, err := v.Rules.Plan(b.Kind())
		if err != nil {
			continue
		}

		businesses = append(businesses, toProtoBusiness(b, plan, v.At))
	}

	return &userv1.BusinessesState{
		Settings:   toProtoBusinessSettings(v.Rules),
		Businesses: businesses,
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
		Action: action,
		Amount: req.GetAmount(),
		Price:  req.GetPrice(),
		Staff:  int(req.GetStaff()),
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
