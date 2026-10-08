// Package clubclient asks the club service who keeps the wall of a
// community clean: its admins delete and pin others' posts and comments
// there. It is the only place in post that knows the club proto contract.
package clubclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

// Client is the service.WallModerators of the club service; each call is
// bounded by timeout.
type Client struct {
	api     clubv1.ClubServiceClient
	timeout time.Duration
}

func New(api clubv1.ClubServiceClient, timeout time.Duration) *Client {
	return &Client{
		api:     api,
		timeout: timeout,
	}
}

func (c *Client) CanModerate(ctx context.Context, wallID, userID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.CanModerateWall(ctx, &clubv1.CanModerateWallRequest{WallId: wallID.String(), UserId: userID.String()})
	if err != nil {
		return false, errs.Wrap("c.api.CanModerateWall", err)
	}

	return resp.GetAllowed(), nil
}

// Nop is the service.WallModerators without a club service: no wall has
// moderators beyond what post itself allows.
type Nop struct{}

func (Nop) CanModerate(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
