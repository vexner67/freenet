// Package clubclient makes the groups of the states in the club service.
package clubclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

// Client implements service.Clubs.
type Client struct {
	api     clubv1.ClubServiceClient
	timeout time.Duration
}

func New(api clubv1.ClubServiceClient, timeout time.Duration) *Client {
	return &Client{api: api, timeout: timeout}
}

func (c *Client) CreateStateClub(ctx context.Context, stateID, ownerID uuid.UUID, name, description string) (uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.CreateStateClub(ctx, &clubv1.CreateStateClubRequest{
		StateId: stateID.String(), OwnerId: ownerID.String(), Name: name, Description: description,
	})
	if err != nil {
		return uuid.Nil, errs.Wrap("c.api.CreateStateClub", err)
	}

	id, err := uuid.Parse(resp.GetClubId())

	return id, errs.Wrap("uuid.Parse", err)
}

// Nop makes no groups: without the club service the states have none.
type Nop struct{}

func (Nop) CreateStateClub(context.Context, uuid.UUID, uuid.UUID, string, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}
