package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeDecorAPI struct {
	userv1.UserServiceClient
	md    metadata.MD
	decor userv1.Decor
}

func (f *fakeDecorAPI) GetDecor(context.Context, *userv1.GetDecorRequest, ...grpc.CallOption) (*userv1.GetDecorResponse, error) {
	return &userv1.GetDecorResponse{Decor: f.decor, Festive: f.decor == userv1.Decor_DECOR_ON}, nil
}

func (f *fakeDecorAPI) SetDecor(ctx context.Context, req *userv1.SetDecorRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.decor = req.GetDecor()

	return &emptypb.Empty{}, nil
}

func TestDecor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeDecorAPI{}
	d := NewDecor(api)

	// A service that says nothing sensible is read as the calendar.
	if got, err := d.Get(ctx); err != nil || got.Mode != "auto" || got.Festive {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	if err := d.Set(ctx, "tok", "on"); err != nil || api.decor != userv1.Decor_DECOR_ON || len(api.md.Get("authorization")) != 1 {
		t.Fatalf("Set: %v, %v, %v", err, api.decor, api.md)
	}
	if got, err := d.Get(ctx); err != nil || got.Mode != "on" || !got.Festive {
		t.Fatalf("Get after = %+v, %v", got, err)
	}

	if err := d.Set(ctx, "tok", "snow"); failure.From(err).Kind != failure.InvalidArgument || api.decor != userv1.Decor_DECOR_ON {
		t.Fatalf("Set snow: %v", err)
	}
}
