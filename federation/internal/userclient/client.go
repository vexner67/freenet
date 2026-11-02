// Package userclient reads the pages of the user service as a guest sees
// them. It is the only place in federation that knows the user proto
// contract.
package userclient

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client is the service.People; each call is bounded by timeout.
type Client struct {
	users   userv1.UserServiceClient
	timeout time.Duration
}

func New(users userv1.UserServiceClient, timeout time.Duration) *Client {
	return &Client{
		users:   users,
		timeout: timeout,
	}
}

// ByID finds the nickname of the page first: only GetProfile tells what
// a guest may see.
func (c *Client) ByID(ctx context.Context, id uuid.UUID) (service.Person, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.users.GetProfiles(ctx, &userv1.GetProfilesRequest{UserIds: []string{id.String()}})
	if err != nil {
		return service.Person{}, errs.Wrap("c.users.GetProfiles", err)
	}

	for _, p := range resp.GetProfiles() {
		if p.GetId() != id.String() {
			continue
		}

		// A page whose profile is not filled in has no address yet.
		if p.GetNickname() == "" {
			return service.Person{ID: id, Hidden: true}, nil
		}

		return c.byNickname(ctx, p.GetNickname())
	}

	return service.Person{}, fedi.ErrNotFound
}

func (c *Client) ByNickname(ctx context.Context, nickname string) (service.Person, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.byNickname(ctx, nickname)
}

func (c *Client) byNickname(ctx context.Context, nickname string) (service.Person, error) {
	resp, err := c.users.GetProfile(ctx, &userv1.GetProfileRequest{Nickname: nickname})
	if status.Code(err) == codes.NotFound || status.Code(err) == codes.InvalidArgument {
		return service.Person{}, fedi.ErrNotFound
	}
	if err != nil {
		return service.Person{}, errs.Wrap("c.users.GetProfile", err)
	}

	p := resp.GetProfile()

	id, err := uuid.Parse(p.GetId())
	if err != nil {
		return service.Person{}, errs.Wrap("uuid.Parse", err)
	}

	return service.Person{
		ID:           id,
		Nickname:     p.GetNickname(),
		Name:         strings.TrimSpace(p.GetName() + " " + p.GetSurname()),
		Status:       p.GetStatus(),
		AvatarID:     p.GetAvatarId(),
		Hidden:       p.GetPageHidden() || p.GetDeactivated(),
		RegisteredAt: p.GetRegisteredAt().AsTime(),
	}, nil
}
