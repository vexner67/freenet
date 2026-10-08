// Package clubclient asks the club service who keeps the wall of a
// community clean: its admins delete and pin others' posts and comments
// there and hear about the posts suggested for it. It is the only place in
// post that knows the club proto contract.
package clubclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

// Client is the service.SuggestionModerators of the club service; each
// call is bounded by timeout.
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

func (c *Client) Moderators(ctx context.Context, wallID uuid.UUID) ([]uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.ListWallModerators(ctx, &clubv1.ListWallModeratorsRequest{WallId: wallID.String()})
	if err != nil {
		return nil, errs.Wrap("c.api.ListWallModerators", err)
	}

	ids := make([]uuid.UUID, 0, len(resp.GetUserIds()))
	for _, s := range resp.GetUserIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, errs.Wrap("uuid.Parse", err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// Nop is the service.SuggestionModerators without a club service: no wall
// has moderators beyond what post itself allows.
type Nop struct{}

func (Nop) CanModerate(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

func (Nop) Moderators(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
