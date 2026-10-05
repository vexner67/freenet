package userclient

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client adapts the user service gRPC API to service.UserProvider.
// It is the only place in auth that knows about the user proto contract.
type Client struct {
	api userv1.UserServiceClient
}

func New(api userv1.UserServiceClient) *Client {
	return &Client{
		api: api,
	}
}

func (c *Client) GetOrCreateByEmail(ctx context.Context, email account.Email) (uuid.UUID, error) {
	resp, err := c.api.GetOrCreateByEmail(ctx, &userv1.GetOrCreateByEmailRequest{
		Email: email.String(),
	})
	if err != nil {
		return uuid.Nil, errs.Wrap("c.api.GetOrCreateByEmail", translateError(err))
	}

	userID, err := uuid.Parse(resp.GetUserId())
	if err != nil {
		return uuid.Nil, errs.Wrap("uuid.Parse", err)
	}

	return userID, nil
}

func translateError(err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return account.ErrInvalidEmail
	case codes.Unavailable, codes.DeadlineExceeded:
		return service.ErrUserServiceUnavailable
	default:
		return err
	}
}
