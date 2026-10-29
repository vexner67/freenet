package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeWorldAPI struct {
	worldv1.WorldServiceClient
}

func (f *fakeWorldAPI) GetWorld(context.Context, *worldv1.GetWorldRequest, ...grpc.CallOption) (*worldv1.GetWorldResponse, error) {
	return &worldv1.GetWorldResponse{
		World: &worldv1.World{Id: "w1", Seed: 7, Grid: 2048, Status: worldv1.WorldStatus_WORLD_STATUS_OPEN},
	}, nil
}

func (f *fakeWorldAPI) EndWorld(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.NotFound, "no world is open")
}

func TestWorld(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := NewWorld(&fakeWorldAPI{})

	got, err := w.Current(ctx)
	if err != nil || got.Status != "open" || got.Seed != 7 || got.Grid != 2048 {
		t.Fatalf("Current = %+v, %v", got, err)
	}

	if err = w.End(ctx); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("End: %v", err)
	}
}
