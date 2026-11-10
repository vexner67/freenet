package userclient

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type Client struct {
	client  userv1.UserServiceClient
	timeout time.Duration
}

func New(client userv1.UserServiceClient, timeout time.Duration) *Client {
	return &Client{client: client, timeout: timeout}
}
func (c *Client) access(ctx context.Context, owner, viewer uuid.UUID) (*userv1.GetRoomAccessResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.client.GetRoomAccess(ctx, &userv1.GetRoomAccessRequest{OwnerId: owner.String(), ViewerId: viewer.String()})
	if status.Code(err) == codes.NotFound {
		return &userv1.GetRoomAccessResponse{Blocked: true, Banned: true}, nil
	}
	if err != nil {
		return nil, errors.Join(room.ErrUnavailable, errs.Wrap("c.client.GetRoomAccess", err))
	}
	return response, nil
}
func (c *Client) Access(ctx context.Context, owner, viewer uuid.UUID) (service.Relationships, error) {
	response, err := c.access(ctx, owner, viewer)
	if err != nil {
		return service.Relationships{}, errs.Wrap("c.access", err)
	}
	return service.Relationships{Friend: response.GetFriend(), Blocked: response.GetBlocked(), Banned: response.GetBanned()}, nil
}
func (c *Client) Blocked(ctx context.Context, viewer uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result := make(map[uuid.UUID]bool)
	for _, id := range ids {
		if _, seen := result[id]; seen {
			continue
		}
		response, err := c.access(ctx, id, viewer)
		if err != nil {
			return nil, errs.Wrap("c.access", err)
		}
		result[id] = response.GetBlocked() || response.GetBanned()
	}
	return result, nil
}
func (c *Client) CanInvite(ctx context.Context, actor, target uuid.UUID) (bool, error) {
	response, err := c.access(ctx, target, actor)
	if err != nil {
		return false, errs.Wrap("c.access", err)
	}
	return response.GetCanInvite() && !response.GetBanned() && !response.GetBlocked(), nil
}
