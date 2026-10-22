package backend

import (
	"context"
	"math"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeMineAPI struct {
	userv1.UserServiceClient
	set *userv1.SetMineRequest
	ore string
	err error
}

func (f *fakeMineAPI) DigOre(_ context.Context, req *userv1.DigOreRequest, _ ...grpc.CallOption) (*userv1.DigOreResponse, error) {
	f.ore = req.GetOre()
	if f.err != nil {
		return nil, f.err
	}

	return &userv1.DigOreResponse{
		Units: 150, Event: "nugget", Experience: 10,
		Mine: &userv1.MineState{
			Settings: &userv1.MineSettings{Energy: 10, Veins: []*userv1.OreVein{{Ore: "stone", Most: 180, Energy: 1}}},
			Miner:    &userv1.Miner{Energy: 9, NextEnergySeconds: 900, Backpack: map[string]int64{"stone": 150}},
		},
	}, nil
}

func (f *fakeMineAPI) SetMine(_ context.Context, req *userv1.SetMineRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.set = req

	return &emptypb.Empty{}, nil
}

func TestMine(t *testing.T) {
	t.Parallel()

	api := &fakeMineAPI{}
	m := NewMine(api)

	dug, err := m.Dig(context.Background(), "tok", "stone")
	if err != nil || api.ore != "stone" || dug.Units != 150 || dug.Event != "nugget" || dug.Mine.Miner.NextEnergySeconds != 900 ||
		dug.Mine.Miner.Backpack["stone"] != 150 || dug.Mine.Settings.Veins[0].Most != 180 {
		t.Fatalf("Dig = %+v, %v", dug, err)
	}

	settings := model.MineSettings{
		Veins:        []model.OreVein{{Ore: "stone", Least: 1, Most: 2, Price: 3, Gain: 4, Energy: 5}},
		Energy:       20,
		RegenSeconds: 60,
		Events:       model.MineEvents{Nugget: 1, NuggetTimes: 2, Chest: 3, ChestLeast: 4, ChestMost: 5, Collapse: 6, Drink: 7, DrinkEnergy: 8},
	}
	if err = m.Set(context.Background(), "tok", settings); err != nil {
		t.Fatal(err)
	}
	if got := fromProtoMineSettings(api.set.GetSettings()); got.Energy != 20 || got.RegenSeconds != 60 || got.Events != settings.Events ||
		got.Veins[0] != settings.Veins[0] {
		t.Fatalf("SetMine got %+v", got)
	}

	// A number past int32 is refused here, as the service would.
	settings.Energy = math.MaxInt32 + 1
	if err = m.Set(context.Background(), "tok", settings); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("Set of a huge energy: %v", err)
	}

	api.err = status.Error(codes.FailedPrecondition, "not enough energy")
	if _, err = m.Dig(context.Background(), "tok", "stone"); failure.From(err).Kind != failure.Conflict {
		t.Fatalf("Dig without energy: %v", err)
	}
}
