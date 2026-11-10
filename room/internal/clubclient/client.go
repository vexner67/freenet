package clubclient

import (
	"context"
	"errors"
	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type Client struct {
	client  clubv1.ClubServiceClient
	timeout time.Duration
}

func New(client clubv1.ClubServiceClient, timeout time.Duration) *Client {
	return &Client{client: client, timeout: timeout}
}
func (c *Client) Access(ctx context.Context, id, user uuid.UUID) (service.ClubAccess, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.client.GetRoomAccess(ctx, &clubv1.GetRoomAccessRequest{ClubId: id.String(), UserId: user.String()})
	if status.Code(err) == codes.NotFound {
		return service.ClubAccess{}, nil
	}
	if err != nil {
		return service.ClubAccess{}, errors.Join(room.ErrUnavailable, errs.Wrap("c.client.GetRoomAccess", err))
	}
	return service.ClubAccess{Member: response.GetMember(), Manager: response.GetManager()}, nil
}
