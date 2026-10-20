package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeCasinoAPI struct {
	userv1.UserServiceClient
	md  metadata.MD
	set *userv1.SetCasinoRequest
	err error
}

func (f *fakeCasinoAPI) GetCasino(context.Context, *userv1.GetCasinoRequest, ...grpc.CallOption) (*userv1.GetCasinoResponse, error) {
	return &userv1.GetCasinoResponse{Price: 10, Prizes: []int64{0, 50}}, nil
}

func (f *fakeCasinoAPI) GetCasinoSettings(ctx context.Context, _ *userv1.GetCasinoSettingsRequest, _ ...grpc.CallOption) (*userv1.GetCasinoSettingsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)

	return &userv1.GetCasinoSettingsResponse{
		Price:  10,
		Prizes: []*userv1.CasinoPrize{{Votes: 0, Weight: 9}, {Votes: 50, Weight: 1}},
		Spins:  3, Spent: 30, Won: 50,
	}, nil
}

func (f *fakeCasinoAPI) SetCasino(ctx context.Context, req *userv1.SetCasinoRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.set = req

	return &emptypb.Empty{}, nil
}

func (f *fakeCasinoAPI) SpinCasino(ctx context.Context, _ *userv1.SpinCasinoRequest, _ ...grpc.CallOption) (*userv1.SpinCasinoResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.SpinCasinoResponse{Prize: 1, Votes: 50, Balance: 60}, nil
}

func TestCasino(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeCasinoAPI{}
	c := NewCasino(api)

	if got, err := c.Get(ctx); err != nil || got.Price != 10 || len(got.Prizes) != 2 || got.Prizes[1].Votes != 50 {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	settings, err := c.Settings(ctx, "tok")
	if err != nil || settings.Casino.Prizes[0].Weight != 9 || settings.Spins != 3 || settings.Won != 50 || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("Settings = %+v, %v, %v", settings, err, api.md)
	}

	if err = c.Set(ctx, "tok", model.Casino{Price: 5, Prizes: []model.CasinoPrize{{Votes: 0, Weight: 1}, {Votes: 7, Weight: 2}}}); err != nil ||
		api.set.GetPrice() != 5 || api.set.GetPrizes()[1].GetWeight() != 2 {
		t.Fatalf("Set: %v, %v", err, api.set)
	}

	if spin, err := c.Spin(ctx, "tok"); err != nil || spin != (model.CasinoSpin{Prize: 1, Votes: 50, Balance: 60}) {
		t.Fatalf("Spin = %+v, %v", spin, err)
	}

	api.err = status.Error(codes.FailedPrecondition, "not enough votes")
	if _, err = c.Spin(ctx, "tok"); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("Spin without votes: %v", err)
	}
}
