package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeWorldAPI struct {
	worldv1.WorldServiceClient
	md    metadata.MD
	found *worldv1.FoundStateRequest
}

func (f *fakeWorldAPI) GetWorld(ctx context.Context, _ *worldv1.GetWorldRequest, _ ...grpc.CallOption) (*worldv1.GetWorldResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)

	return &worldv1.GetWorldResponse{
		World:  &worldv1.World{Id: "w1", Seed: 7, Grid: 2048, Status: worldv1.WorldStatus_WORLD_STATUS_OPEN},
		States: []*worldv1.State{{Id: "s1", Name: "Ruritania", Flag: &worldv1.Flag{Pattern: "tricolor_v", Colors: []int32{1, 2, 3}}}},
		Owners: []int32{0, 1},
		Viewer: &worldv1.Standing{AppliedTo: "s1"},
	}, nil
}

func (f *fakeWorldAPI) FoundState(_ context.Context, req *worldv1.FoundStateRequest, _ ...grpc.CallOption) (*worldv1.FoundStateResponse, error) {
	f.found = req
	return &worldv1.FoundStateResponse{StateId: "s2"}, nil
}

func (f *fakeWorldAPI) LeaveState(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.FailedPrecondition, "the ruler cannot leave the state")
}

func TestWorld(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeWorldAPI{}
	w := NewWorld(api)

	m, err := w.Map(ctx, "tok")
	if err != nil || m.World.Status != "open" || m.World.Seed != 7 || m.States[0].Flag.Colors[2] != 3 || m.Owners[1] != 1 ||
		m.Viewer.AppliedTo != "s1" || len(api.md.Get("authorization")) != 1 {
		t.Fatalf("Map = %+v, %v", m, err)
	}

	id, err := w.Found(ctx, "tok", model.Founding{Name: "Ruritania", Capital: 4, Flag: model.Flag{Pattern: "plain", Colors: []int{5}, Emblem: "star"}})
	if err != nil || id != "s2" || api.found.GetCapital() != 4 || api.found.GetFlag().GetColors()[0] != 5 || api.found.GetFlag().GetEmblem() != "star" {
		t.Fatalf("Found = %q, %v, %v", id, err, api.found)
	}

	if err = w.Leave(ctx, "tok"); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("Leave: %v", err)
	}
}
