// Package userclient takes and pays the votes of pages in the user
// service.
package userclient

import (
	"context"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/world/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notEnoughVotes is the message of the user service for a page short of
// votes; its other FailedPrecondition is a page too new.
const notEnoughVotes = "not enough votes"

// Client implements service.Wallets.
type Client struct {
	api     userv1.UserServiceClient
	timeout time.Duration
}

func New(api userv1.UserServiceClient, timeout time.Duration) *Client {
	return &Client{api: api, timeout: timeout}
}

func (c *Client) Charge(ctx context.Context, op service.VoteOp) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.ChargeVotes(ctx, &userv1.ChargeVotesRequest{
		OpId: op.OpID.String(), UserId: op.UserID.String(), Votes: op.Votes, Note: op.Note, Settled: op.Settled,
	})
	if err != nil {
		return 0, errs.Wrap("c.api.ChargeVotes", fromStatus(err))
	}

	return resp.GetVotes(), nil
}

func (c *Client) Pay(ctx context.Context, op service.VoteOp) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.PayVotes(ctx, &userv1.PayVotesRequest{
		OpId: op.OpID.String(), UserId: op.UserID.String(), Votes: op.Votes, Note: op.Note,
	})
	if err != nil {
		return 0, errs.Wrap("c.api.PayVotes", fromStatus(err))
	}

	return resp.GetVotes(), nil
}

// fromStatus turns what the user service refused into the errors of the
// service; anything else stays as it is.
func fromStatus(err error) error {
	s, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch s.Code() {
	case codes.FailedPrecondition:
		if s.Message() == notEnoughVotes {
			return service.ErrNotEnoughVotes
		}
		return service.ErrNotSettled
	case codes.PermissionDenied, codes.NotFound:
		return service.ErrPageGone
	default:
		return err
	}
}
