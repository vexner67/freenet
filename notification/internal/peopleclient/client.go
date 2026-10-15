// Package peopleclient names who did what a push tells of: a person from
// the user service or, failing that, a community from the club service.
// It is the only place in notification that knows their proto contracts.
package peopleclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

var genders = map[userv1.Gender]string{
	userv1.Gender_GENDER_MALE:   "male",
	userv1.Gender_GENDER_FEMALE: "female",
}

// Client is the service.Actors; each call is bounded by timeout. Without
// a club service (clubs nil) communities stay unnamed.
type Client struct {
	users   userv1.UserServiceClient
	clubs   clubv1.ClubServiceClient
	timeout time.Duration
}

func New(users userv1.UserServiceClient, clubs clubv1.ClubServiceClient, timeout time.Duration) *Client {
	return &Client{
		users:   users,
		clubs:   clubs,
		timeout: timeout,
	}
}

func (c *Client) Actors(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]push.Actor, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	people, err := c.users.GetProfiles(ctx, &userv1.GetProfilesRequest{UserIds: toStrings(ids)})
	if err != nil {
		return nil, errs.Wrap("c.users.GetProfiles", err)
	}

	actors := make(map[uuid.UUID]push.Actor, len(ids))

	for _, p := range people.GetProfiles() {
		id, err := uuid.Parse(p.GetId())
		if err != nil {
			continue
		}

		actors[id] = push.Actor{
			ID:       id,
			Name:     p.GetName() + " " + p.GetSurname(),
			Gender:   genders[p.GetGender()],
			Nickname: p.GetNickname(),
			AvatarID: p.GetAvatarId(),
		}
	}

	var rest []uuid.UUID

	for _, id := range ids {
		if _, ok := actors[id]; !ok {
			rest = append(rest, id)
		}
	}

	if len(rest) == 0 || c.clubs == nil {
		return actors, nil
	}

	clubs, err := c.clubs.GetClubs(ctx, &clubv1.GetClubsRequest{Ids: toStrings(rest)})
	if err != nil {
		return nil, errs.Wrap("c.clubs.GetClubs", err)
	}

	for _, cl := range clubs.GetClubs() {
		id, err := uuid.Parse(cl.GetId())
		if err != nil {
			continue
		}

		actors[id] = push.Actor{
			ID:       id,
			Name:     cl.GetName(),
			Nickname: cl.GetAddress(),
			Club:     true,
			AvatarID: cl.GetAvatarId(),
		}
	}

	return actors, nil
}

func toStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

// Nop is the service.Actors without a user service: pushes say what
// happened without who.
type Nop struct{}

func (Nop) Actors(context.Context, []uuid.UUID) (map[uuid.UUID]push.Actor, error) {
	return nil, nil
}
