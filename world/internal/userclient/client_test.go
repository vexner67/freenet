package userclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/userclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAPI answers ChargeVotes with err.
type fakeAPI struct {
	userv1.UserServiceClient
	err error
}

func (f fakeAPI) ChargeVotes(context.Context, *userv1.ChargeVotesRequest, ...grpc.CallOption) (*userv1.ChargeVotesResponse, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.ChargeVotesResponse{Votes: 7}, nil
}

func TestCharge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"short of votes", status.Error(codes.FailedPrecondition, "not enough votes"), service.ErrNotEnoughVotes},
		{"too new", status.Error(codes.FailedPrecondition, "the page cannot run enterprises yet"), service.ErrNotSettled},
		{"deleted", status.Error(codes.PermissionDenied, "this page is deleted"), service.ErrPageGone},
		{"erased", status.Error(codes.NotFound, "user not found"), service.ErrPageGone},
	}
	op := service.VoteOp{OpID: uuid.New(), UserID: uuid.New(), Votes: 5}
	for _, tt := range tests {
		c := userclient.New(fakeAPI{err: tt.err}, time.Second)
		if _, err := c.Charge(context.Background(), op); !errors.Is(err, tt.want) {
			t.Errorf("%s: Charge = %v, want %v", tt.name, err, tt.want)
		}
	}

	down := status.Error(codes.Unavailable, "down")
	if _, err := userclient.New(fakeAPI{err: down}, time.Second).Charge(context.Background(), op); status.Code(errors.Unwrap(err)) != codes.Unavailable {
		t.Errorf("Charge when user is down = %v", err)
	}
	if left, err := userclient.New(fakeAPI{}, time.Second).Charge(context.Background(), op); err != nil || left != 7 {
		t.Errorf("Charge = %d, %v", left, err)
	}
}
