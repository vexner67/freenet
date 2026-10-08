package clubclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/post/internal/clubclient"
	"google.golang.org/grpc"
)

type fakeClub struct {
	clubv1.ClubServiceClient
	req        *clubv1.CanModerateWallRequest
	moderators []string
	deadline   bool
	err        error
}

func (f *fakeClub) ListWallModerators(ctx context.Context, req *clubv1.ListWallModeratorsRequest, _ ...grpc.CallOption) (*clubv1.ListWallModeratorsResponse, error) {
	_, f.deadline = ctx.Deadline()
	return &clubv1.ListWallModeratorsResponse{UserIds: f.moderators}, f.err
}

func (f *fakeClub) CanModerateWall(ctx context.Context, req *clubv1.CanModerateWallRequest, _ ...grpc.CallOption) (*clubv1.CanModerateWallResponse, error) {
	f.req = req
	_, f.deadline = ctx.Deadline()
	return &clubv1.CanModerateWallResponse{Allowed: true}, f.err
}

func TestClient(t *testing.T) {
	t.Parallel()

	wall, user := uuid.New(), uuid.New()
	api := &fakeClub{}

	ok, err := clubclient.New(api, time.Second).CanModerate(context.Background(), wall, user)
	if err != nil || !ok {
		t.Fatalf("CanModerate = %v, %v", ok, err)
	}
	if api.req.GetWallId() != wall.String() || api.req.GetUserId() != user.String() || !api.deadline {
		t.Fatalf("request = %v (deadline %v)", api.req, api.deadline)
	}

	api.err = errors.New("down")
	if _, err = clubclient.New(api, time.Second).CanModerate(context.Background(), wall, user); err == nil {
		t.Fatal("an unreachable club service is an error, not a no")
	}

	if ok, err = (clubclient.Nop{}).CanModerate(context.Background(), wall, user); err != nil || ok {
		t.Fatalf("Nop = %v, %v", ok, err)
	}
}

func TestClient_Moderators(t *testing.T) {
	t.Parallel()

	wall, admin := uuid.New(), uuid.New()
	api := &fakeClub{moderators: []string{admin.String()}}

	ids, err := clubclient.New(api, time.Second).Moderators(context.Background(), wall)
	if err != nil || len(ids) != 1 || ids[0] != admin || !api.deadline {
		t.Fatalf("Moderators = %v, %v (deadline %v)", ids, err, api.deadline)
	}

	api.moderators = []string{"nobody"}
	if _, err = clubclient.New(api, time.Second).Moderators(context.Background(), wall); err == nil {
		t.Fatal("an invalid ID from the club service is accepted")
	}

	api.err = errors.New("down")
	if _, err = clubclient.New(api, time.Second).Moderators(context.Background(), wall); err == nil {
		t.Fatal("an unreachable club service is an error, not nobody")
	}

	if ids, err = (clubclient.Nop{}).Moderators(context.Background(), wall); err != nil || len(ids) != 0 {
		t.Fatalf("Nop = %v, %v", ids, err)
	}
}
