package backend

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeBetsAPI struct {
	userv1.UserServiceClient
	md      metadata.MD
	list    *userv1.ListBetEventsRequest
	create  *userv1.CreateBetEventRequest
	resolve *userv1.ResolveBetEventRequest
	stake   *userv1.StakeBetRequest
	err     error
}

var closesAt = time.Date(2026, 11, 3, 18, 0, 0, 0, time.UTC)

func (f *fakeBetsAPI) ListBetEvents(ctx context.Context, req *userv1.ListBetEventsRequest, _ ...grpc.CallOption) (*userv1.ListBetEventsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.list = req

	return &userv1.ListBetEventsResponse{Events: []*userv1.BetEvent{{
		Id: "e1", Question: "Q?", State: "open", Open: true, Winner: -1, People: 2, ClosesAt: timestamppb.New(closesAt),
		Outcomes: []*userv1.BetOutcome{{Name: "Да", Staked: 5, Bettors: 1, Mine: 5}, {Name: "Нет"}},
	}}}, nil
}

func (f *fakeBetsAPI) CreateBetEvent(_ context.Context, req *userv1.CreateBetEventRequest, _ ...grpc.CallOption) (*userv1.BetEvent, error) {
	f.create = req

	return &userv1.BetEvent{Id: "e2", Question: req.GetQuestion(), Winner: -1}, nil
}

func (f *fakeBetsAPI) ResolveBetEvent(_ context.Context, req *userv1.ResolveBetEventRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.resolve = req

	return &emptypb.Empty{}, f.err
}

func (f *fakeBetsAPI) StakeBet(_ context.Context, req *userv1.StakeBetRequest, _ ...grpc.CallOption) (*userv1.StakeBetResponse, error) {
	f.stake = req
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.StakeBetResponse{Balance: 7}, nil
}

func TestBets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeBetsAPI{}
	b := NewBets(api)

	events, err := b.List(ctx, "", true)
	if err != nil || len(events) != 1 || !api.list.GetPopular() || len(api.md.Get("authorization")) != 0 {
		t.Fatalf("List = %+v, %v", events, err)
	}
	if e := events[0]; e.People != 2 || !e.ClosesAt.Equal(closesAt) || e.Outcomes[0] != (model.BetOutcome{Name: "Да", Staked: 5, Bettors: 1, Mine: 5}) {
		t.Fatalf("event = %+v", e)
	}

	if _, err = b.Create(ctx, "tok", model.BetEventDraft{Question: "Q?", Outcomes: []string{"a", "b"}}); err != nil || api.create.GetClosesAt() != nil {
		t.Fatalf("Create without a deadline: %v, %v", err, api.create)
	}
	if _, err = b.Create(ctx, "tok", model.BetEventDraft{Question: "Q?", Outcomes: []string{"a", "b"}, ClosesAt: closesAt}); err != nil ||
		!api.create.GetClosesAt().AsTime().Equal(closesAt) {
		t.Fatalf("Create: %v, %v", err, api.create)
	}

	if err = b.Resolve(ctx, "tok", "e1", -1); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("Resolve(-1): %v", err)
	}
	if err = b.Resolve(ctx, "tok", "e1", 1); err != nil || api.resolve.GetWinner() != 1 {
		t.Fatalf("Resolve: %v, %v", err, api.resolve)
	}

	if balance, err := b.Stake(ctx, "tok", "e1", 0, 3); err != nil || balance != 7 || api.stake.GetAmount() != 3 {
		t.Fatalf("Stake = %d, %v", balance, err)
	}
	api.err = status.Error(codes.FailedPrecondition, "the event takes no more stakes")
	if _, err = b.Stake(ctx, "tok", "e1", 0, 3); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("Stake on a closed event: %v", err)
	}
}
