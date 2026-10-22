package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MineService interface {
	Mine(ctx context.Context, userID uuid.UUID, now time.Time) (service.MineView, error)
	Dig(ctx context.Context, userID uuid.UUID, ore user.Ore, now time.Time) (service.Dug, error)
	Sell(ctx context.Context, userID uuid.UUID, ore user.Ore, now time.Time) (service.Sold, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.MineSettings, error)
	SetMine(ctx context.Context, adminID uuid.UUID, mine user.Mine) error
}

func toProtoMineSettings(m user.Mine) *userv1.MineSettings {
	veins := make([]*userv1.OreVein, 0, len(m.Veins()))
	for _, v := range m.Veins() {
		veins = append(veins, &userv1.OreVein{
			Ore:        v.Ore.String(),
			Experience: v.Experience,
			Least:      v.Least,
			Most:       v.Most,
			Price:      v.Price,
			Gain:       v.Gain,
			Energy:     int32(v.Energy), //nolint:gosec // at most user.MaxOreEnergy
		})
	}

	e := m.Events()

	return &userv1.MineSettings{
		Veins:        veins,
		Energy:       int32(m.Energy()),              //nolint:gosec // at most user.MaxMineEnergy
		RegenSeconds: int32(m.Regen() / time.Second), //nolint:gosec // at most user.MaxMineRegen
		Events: &userv1.MineEvents{
			Nugget:      int32(e.Nugget),      //nolint:gosec // at most user.MineOdds
			NuggetTimes: int32(e.NuggetTimes), //nolint:gosec // at most user.MaxNuggetTimes
			Chest:       int32(e.Chest),       //nolint:gosec // at most user.MineOdds
			ChestLeast:  e.ChestLeast,
			ChestMost:   e.ChestMost,
			Collapse:    int32(e.Collapse),    //nolint:gosec // at most user.MineOdds
			Drink:       int32(e.Drink),       //nolint:gosec // at most user.MineOdds
			DrinkEnergy: int32(e.DrinkEnergy), //nolint:gosec // at most user.MaxDrinkEnergy
		},
	}
}

func fromProtoMineSettings(s *userv1.MineSettings) (user.Mine, error) {
	veins := make([]user.OreVein, 0, len(s.GetVeins()))
	for _, v := range s.GetVeins() {
		veins = append(veins, user.OreVein{
			Ore:        user.Ore(v.GetOre()),
			Experience: v.GetExperience(),
			Least:      v.GetLeast(),
			Most:       v.GetMost(),
			Price:      v.GetPrice(),
			Gain:       v.GetGain(),
			Energy:     int(v.GetEnergy()),
		})
	}

	e := s.GetEvents()

	return user.NewMine(veins, int(s.GetEnergy()), time.Duration(s.GetRegenSeconds())*time.Second, user.MineEvents{
		Nugget:      int(e.GetNugget()),
		NuggetTimes: int(e.GetNuggetTimes()),
		Chest:       int(e.GetChest()),
		ChestLeast:  e.GetChestLeast(),
		ChestMost:   e.GetChestMost(),
		Collapse:    int(e.GetCollapse()),
		Drink:       int(e.GetDrink()),
		DrinkEnergy: int(e.GetDrinkEnergy()),
	})
}

func toProtoMineState(v service.MineView) *userv1.MineState {
	backpack := map[string]int64{}
	for o, units := range v.Miner.Backpack() {
		backpack[o.String()] = units
	}

	return &userv1.MineState{
		Settings: toProtoMineSettings(v.Mine),
		Miner: &userv1.Miner{
			Energy:            int32(v.Miner.Energy()),                               //nolint:gosec // at most user.MaxMineEnergy
			NextEnergySeconds: int32((v.NextEnergy + time.Second - 1) / time.Second), //nolint:gosec // at most user.MaxMineRegen
			Experience:        v.Miner.Experience(),
			Backpack:          backpack,
			Digs:              v.Miner.Digs(),
			Earned:            v.Miner.Earned(),
		},
	}
}

func (h *UserHandler) GetMine(ctx context.Context, _ *userv1.GetMineRequest) (*userv1.MineState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	v, err := h.mines.Mine(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	return toProtoMineState(v), nil
}

func (h *UserHandler) DigOre(ctx context.Context, req *userv1.DigOreRequest) (*userv1.DigOreResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ore, err := user.NewOre(req.GetOre())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	dug, err := h.mines.Dig(ctx, userID, ore, time.Now())
	if err != nil {
		return nil, err
	}

	return &userv1.DigOreResponse{
		Units:      dug.Haul.Units,
		Event:      string(dug.Haul.Event),
		Votes:      dug.Haul.Votes,
		Balance:    dug.Balance,
		Experience: dug.Haul.Experience,
		Mine:       toProtoMineState(dug.View),
	}, nil
}

func (h *UserHandler) SellOre(ctx context.Context, req *userv1.SellOreRequest) (*userv1.SellOreResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var ore user.Ore
	if req.GetOre() != "" {
		if ore, err = user.NewOre(req.GetOre()); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sold, err := h.mines.Sell(ctx, userID, ore, time.Now())
	if err != nil {
		return nil, err
	}

	return &userv1.SellOreResponse{
		Units:   sold.Units,
		Votes:   sold.Votes,
		Balance: sold.Balance,
		Mine:    toProtoMineState(sold.View),
	}, nil
}

func (h *UserHandler) GetMineSettings(ctx context.Context, _ *userv1.GetMineSettingsRequest) (*userv1.GetMineSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.mines.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetMineSettingsResponse{
		Settings: toProtoMineSettings(s.Mine),
		Digs:     s.Stats.Digs,
		Sold:     s.Stats.Sold,
		Chests:   s.Stats.Chests,
		Miners:   s.Stats.Miners,
	}, nil
}

func (h *UserHandler) SetMine(ctx context.Context, req *userv1.SetMineRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	mine, err := fromProtoMineSettings(req.GetSettings())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.mines.SetMine(ctx, adminID, mine); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
