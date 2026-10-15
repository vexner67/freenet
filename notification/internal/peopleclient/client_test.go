package peopleclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/notification/internal/peopleclient"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
)

type users struct {
	userv1.UserServiceClient
	profiles []*userv1.PublicProfile
}

func (u users) GetProfiles(context.Context, *userv1.GetProfilesRequest, ...grpc.CallOption) (*userv1.GetProfilesResponse, error) {
	return &userv1.GetProfilesResponse{Profiles: u.profiles}, nil
}

type clubs struct {
	clubv1.ClubServiceClient
	asked *[]string
}

func (c clubs) GetClubs(_ context.Context, req *clubv1.GetClubsRequest, _ ...grpc.CallOption) (*clubv1.GetClubsResponse, error) {
	*c.asked = req.GetIds()

	return &clubv1.GetClubsResponse{Clubs: []*clubv1.Club{{Id: req.GetIds()[0], Name: "Кошки", Address: "cats", AvatarId: "a1"}}}, nil
}

func TestActors(t *testing.T) {
	t.Parallel()

	ann, cats := uuid.New(), uuid.New()
	var asked []string

	c := peopleclient.New(
		users{profiles: []*userv1.PublicProfile{{Id: ann.String(), Name: "Анна", Surname: "Иванова", Nickname: "ann", Gender: userv1.Gender_GENDER_FEMALE}}},
		clubs{asked: &asked},
		time.Second,
	)

	actors, err := c.Actors(context.Background(), []uuid.UUID{ann, cats})
	if err != nil {
		t.Fatal(err)
	}

	if a := actors[ann]; a.Name != "Анна Иванова" || a.Gender != "female" || a.Nickname != "ann" || a.Club {
		t.Fatalf("ann = %+v", a)
	}

	if a := actors[cats]; a.Name != "Кошки" || !a.Club || a.Nickname != "cats" || a.AvatarID != "a1" {
		t.Fatalf("cats = %+v", a)
	}

	// Only who is not a person is looked for among communities.
	if len(asked) != 1 || asked[0] != cats.String() {
		t.Fatalf("clubs asked for %v", asked)
	}
}
