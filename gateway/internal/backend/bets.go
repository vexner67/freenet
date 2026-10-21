package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Bets are the bets on events, which the user service keeps with the
// votes.
type Bets struct {
	api userv1.UserServiceClient
}

func NewBets(api userv1.UserServiceClient) *Bets {
	return &Bets{
		api: api,
	}
}

// List is the events with the stakes of the viewer; accessToken is
// optional.
func (b *Bets) List(ctx context.Context, accessToken string, popular bool) ([]model.BetEvent, error) {
	resp, err := b.api.ListBetEvents(withToken(ctx, accessToken), &userv1.ListBetEventsRequest{Popular: popular})
	if err != nil {
		return nil, errs.Wrap("b.api.ListBetEvents", fromStatus(err))
	}

	events := make([]model.BetEvent, 0, len(resp.GetEvents()))
	for _, e := range resp.GetEvents() {
		events = append(events, fromProtoBetEvent(e))
	}

	return events, nil
}

func (b *Bets) Create(ctx context.Context, accessToken string, draft model.BetEventDraft) (model.BetEvent, error) {
	odds, err := toProtoOdds(draft.Odds)
	if err != nil {
		return model.BetEvent{}, err
	}

	req := &userv1.CreateBetEventRequest{Question: draft.Question, Outcomes: draft.Outcomes, Odds: odds}
	if !draft.ClosesAt.IsZero() {
		req.ClosesAt = timestamppb.New(draft.ClosesAt)
	}

	resp, err := b.api.CreateBetEvent(withToken(ctx, accessToken), req)
	if err != nil {
		return model.BetEvent{}, errs.Wrap("b.api.CreateBetEvent", fromStatus(err))
	}

	return fromProtoBetEvent(resp), nil
}

// SetOdds changes the odds of the outcomes for the stakes from now on.
func (b *Bets) SetOdds(ctx context.Context, accessToken, id string, odds []int) error {
	req, err := toProtoOdds(odds)
	if err != nil {
		return err
	}

	_, err = b.api.SetBetEventOdds(withToken(ctx, accessToken), &userv1.SetBetEventOddsRequest{EventId: id, Odds: req})

	return errs.Wrap("b.api.SetBetEventOdds", fromStatus(err))
}

// toProtoOdds are odds the user service can take; more of them than an
// event has outcomes it refuses itself.
func toProtoOdds(odds []int) ([]int32, error) {
	out := make([]int32, 0, len(odds))
	for _, o := range odds {
		if o < 0 || o > math.MaxInt32 {
			return nil, failure.New(failure.InvalidArgument, "the odds of an outcome must be from 1.01 to 1000")
		}
		out = append(out, int32(o))
	}

	return out, nil
}

func (b *Bets) Close(ctx context.Context, accessToken, id string) error {
	_, err := b.api.CloseBetEvent(withToken(ctx, accessToken), &userv1.BetEventRequest{EventId: id})

	return errs.Wrap("b.api.CloseBetEvent", fromStatus(err))
}

func (b *Bets) Resolve(ctx context.Context, accessToken, id string, winner int) error {
	if winner < 0 || winner > math.MaxInt32 {
		return failure.New(failure.InvalidArgument, "unknown outcome")
	}

	_, err := b.api.ResolveBetEvent(withToken(ctx, accessToken), &userv1.ResolveBetEventRequest{EventId: id, Winner: int32(winner)})

	return errs.Wrap("b.api.ResolveBetEvent", fromStatus(err))
}

func (b *Bets) Cancel(ctx context.Context, accessToken, id string) error {
	_, err := b.api.CancelBetEvent(withToken(ctx, accessToken), &userv1.BetEventRequest{EventId: id})

	return errs.Wrap("b.api.CancelBetEvent", fromStatus(err))
}

func (b *Bets) Delete(ctx context.Context, accessToken, id string) error {
	_, err := b.api.DeleteBetEvent(withToken(ctx, accessToken), &userv1.BetEventRequest{EventId: id})

	return errs.Wrap("b.api.DeleteBetEvent", fromStatus(err))
}

// Stake puts amount votes on the outcome and returns the balance left.
func (b *Bets) Stake(ctx context.Context, accessToken, id string, outcome int, amount int64) (int64, error) {
	if outcome < 0 || outcome > math.MaxInt32 {
		return 0, failure.New(failure.InvalidArgument, "unknown outcome")
	}

	resp, err := b.api.StakeBet(withToken(ctx, accessToken), &userv1.StakeBetRequest{EventId: id, Outcome: int32(outcome), Amount: amount})
	if err != nil {
		return 0, errs.Wrap("b.api.StakeBet", fromStatus(err))
	}

	return resp.GetBalance(), nil
}

func fromProtoBetEvent(e *userv1.BetEvent) model.BetEvent {
	out := model.BetEvent{
		ID:        e.GetId(),
		Question:  e.GetQuestion(),
		State:     e.GetState(),
		Open:      e.GetOpen(),
		Winner:    int(e.GetWinner()),
		CreatedAt: e.GetCreatedAt().AsTime(),
		People:    int(e.GetPeople()),
		Payout:    e.GetPayout(),
	}
	if e.GetClosesAt() != nil {
		out.ClosesAt = e.GetClosesAt().AsTime()
	}

	for _, o := range e.GetOutcomes() {
		out.Outcomes = append(out.Outcomes, model.BetOutcome{
			Name:      o.GetName(),
			Odds:      int(o.GetOdds()),
			Staked:    o.GetStaked(),
			Bettors:   int(o.GetBettors()),
			Mine:      o.GetMine(),
			Potential: o.GetPotential(),
		})
	}

	return out
}
