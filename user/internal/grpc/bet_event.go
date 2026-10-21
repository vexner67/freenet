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

type BetEventService interface {
	Events(ctx context.Context, viewerID uuid.UUID, list service.BetEventList) ([]service.BetEventView, error)
	Create(ctx context.Context, adminID uuid.UUID, question user.BetQuestion, outcomes []user.BetOutcome, odds []int, closesAt time.Time) (*user.BetEvent, error)
	SetOdds(ctx context.Context, adminID, id uuid.UUID, odds []int) error
	Close(ctx context.Context, adminID, id uuid.UUID) error
	Resolve(ctx context.Context, adminID, id uuid.UUID, winner int) error
	Cancel(ctx context.Context, adminID, id uuid.UUID) error
	Delete(ctx context.Context, adminID, id uuid.UUID) error
	Stake(ctx context.Context, userID, id uuid.UUID, outcome int, amount int64) (int64, error)
}

func (h *UserHandler) ListBetEvents(ctx context.Context, req *userv1.ListBetEventsRequest) (*userv1.ListBetEventsResponse, error) {
	list := service.BetEventsAll
	if req.GetPopular() {
		list = service.BetEventsPopular
	}

	views, err := h.betEvents.Events(ctx, viewerID(ctx), list)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListBetEventsResponse{Events: make([]*userv1.BetEvent, 0, len(views))}
	for _, v := range views {
		resp.Events = append(resp.Events, toProtoBetEvent(v))
	}

	return resp, nil
}

func (h *UserHandler) CreateBetEvent(ctx context.Context, req *userv1.CreateBetEventRequest) (*userv1.BetEvent, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	question, err := user.NewBetQuestion(req.GetQuestion())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if len(req.GetOutcomes()) > user.MaxBetOutcomes {
		return nil, status.Error(codes.InvalidArgument, user.ErrBetOutcomes.Error())
	}
	outcomes := make([]user.BetOutcome, 0, len(req.GetOutcomes()))
	for _, name := range req.GetOutcomes() {
		o, err := user.NewBetOutcome(name)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		outcomes = append(outcomes, o)
	}

	var closesAt time.Time
	if req.GetClosesAt() != nil {
		closesAt = req.GetClosesAt().AsTime()
	}

	e, err := h.betEvents.Create(ctx, adminID, question, outcomes, fromProtoOdds(req.GetOdds()), closesAt)
	if err != nil {
		return nil, err
	}

	n := len(e.Outcomes())

	return toProtoBetEvent(service.BetEventView{
		Event: e, Staked: make([]int64, n), Bettors: make([]int, n), Mine: make([]int64, n), Potential: make([]int64, n), Open: true,
	}), nil
}

func (h *UserHandler) CloseBetEvent(ctx context.Context, req *userv1.BetEventRequest) (*emptypb.Empty, error) {
	return h.changeBetEvent(ctx, req.GetEventId(), h.betEvents.Close)
}

func (h *UserHandler) CancelBetEvent(ctx context.Context, req *userv1.BetEventRequest) (*emptypb.Empty, error) {
	return h.changeBetEvent(ctx, req.GetEventId(), h.betEvents.Cancel)
}

func (h *UserHandler) DeleteBetEvent(ctx context.Context, req *userv1.BetEventRequest) (*emptypb.Empty, error) {
	return h.changeBetEvent(ctx, req.GetEventId(), h.betEvents.Delete)
}

func (h *UserHandler) SetBetEventOdds(ctx context.Context, req *userv1.SetBetEventOddsRequest) (*emptypb.Empty, error) {
	return h.changeBetEvent(ctx, req.GetEventId(), func(ctx context.Context, adminID, id uuid.UUID) error {
		return h.betEvents.SetOdds(ctx, adminID, id, fromProtoOdds(req.GetOdds()))
	})
}

// fromProtoOdds are the odds of a request; more of them than outcomes can
// be is refused by the domain all the same.
func fromProtoOdds(odds []int32) []int {
	out := make([]int, 0, len(odds))
	for _, o := range odds {
		out = append(out, int(o))
	}

	return out
}

func (h *UserHandler) ResolveBetEvent(ctx context.Context, req *userv1.ResolveBetEventRequest) (*emptypb.Empty, error) {
	return h.changeBetEvent(ctx, req.GetEventId(), func(ctx context.Context, adminID, id uuid.UUID) error {
		return h.betEvents.Resolve(ctx, adminID, id, int(req.GetWinner()))
	})
}

func (h *UserHandler) changeBetEvent(ctx context.Context, eventID string, change func(ctx context.Context, adminID, id uuid.UUID) error) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(eventID, "event id")
	if err != nil {
		return nil, err
	}

	if err = change(ctx, adminID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) StakeBet(ctx context.Context, req *userv1.StakeBetRequest) (*userv1.StakeBetResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetEventId(), "event id")
	if err != nil {
		return nil, err
	}

	balance, err := h.betEvents.Stake(ctx, userID, id, int(req.GetOutcome()), req.GetAmount())
	if err != nil {
		return nil, err
	}

	return &userv1.StakeBetResponse{Balance: balance}, nil
}

func toProtoBetEvent(v service.BetEventView) *userv1.BetEvent {
	e := v.Event
	out := &userv1.BetEvent{
		Id:        e.ID().String(),
		Question:  e.Question().String(),
		State:     string(e.State()),
		Winner:    int32(e.Winner()), //nolint:gosec // below user.MaxBetOutcomes
		CreatedAt: timestamppb.New(e.CreatedAt()),
		Open:      v.Open,
		People:    int32(v.People), //nolint:gosec // a count of people
		Payout:    v.Payout,
	}
	if !e.ClosesAt().IsZero() {
		out.ClosesAt = timestamppb.New(e.ClosesAt())
	}

	odds := e.Odds()
	for i, o := range e.Outcomes() {
		outcome := &userv1.BetOutcome{
			Name:    o.String(),
			Staked:  v.Staked[i],
			Bettors: int32(v.Bettors[i]), //nolint:gosec // a count of people
			Mine:    v.Mine[i],
		}
		if i < len(odds) {
			outcome.Odds = int32(odds[i]) //nolint:gosec // at most user.MaxEventOdds
		}
		if i < len(v.Potential) {
			outcome.Potential = v.Potential[i]
		}
		out.Outcomes = append(out.Outcomes, outcome)
	}

	return out
}
