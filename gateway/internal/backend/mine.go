package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Mine is the mine of the site, which the user service keeps with the
// votes.
type Mine struct {
	api userv1.UserServiceClient
}

func NewMine(api userv1.UserServiceClient) *Mine {
	return &Mine{
		api: api,
	}
}

func (m *Mine) Get(ctx context.Context, accessToken string) (model.Mine, error) {
	resp, err := m.api.GetMine(withToken(ctx, accessToken), &userv1.GetMineRequest{})
	if err != nil {
		return model.Mine{}, errs.Wrap("m.api.GetMine", fromStatus(err))
	}

	return fromProtoMine(resp), nil
}

func (m *Mine) Dig(ctx context.Context, accessToken, ore string) (model.MineDig, error) {
	resp, err := m.api.DigOre(withToken(ctx, accessToken), &userv1.DigOreRequest{Ore: ore})
	if err != nil {
		return model.MineDig{}, errs.Wrap("m.api.DigOre", fromStatus(err))
	}

	return model.MineDig{
		Units:      resp.GetUnits(),
		Event:      resp.GetEvent(),
		Votes:      resp.GetVotes(),
		Balance:    resp.GetBalance(),
		Experience: resp.GetExperience(),
		Mine:       fromProtoMine(resp.GetMine()),
	}, nil
}

func (m *Mine) Sell(ctx context.Context, accessToken, ore string) (model.MineSale, error) {
	resp, err := m.api.SellOre(withToken(ctx, accessToken), &userv1.SellOreRequest{Ore: ore})
	if err != nil {
		return model.MineSale{}, errs.Wrap("m.api.SellOre", fromStatus(err))
	}

	return model.MineSale{
		Units:   resp.GetUnits(),
		Votes:   resp.GetVotes(),
		Balance: resp.GetBalance(),
		Mine:    fromProtoMine(resp.GetMine()),
	}, nil
}

func (m *Mine) Settings(ctx context.Context, accessToken string) (model.MineSettings, model.MineStats, error) {
	resp, err := m.api.GetMineSettings(withToken(ctx, accessToken), &userv1.GetMineSettingsRequest{})
	if err != nil {
		return model.MineSettings{}, model.MineStats{}, errs.Wrap("m.api.GetMineSettings", fromStatus(err))
	}

	stats := model.MineStats{Digs: resp.GetDigs(), Sold: resp.GetSold(), Chests: resp.GetChests(), Miners: resp.GetMiners()}

	return fromProtoMineSettings(resp.GetSettings()), stats, nil
}

func (m *Mine) Set(ctx context.Context, accessToken string, settings model.MineSettings) error {
	req, err := toProtoMineSettings(settings)
	if err != nil {
		return err
	}

	if _, err = m.api.SetMine(withToken(ctx, accessToken), &userv1.SetMineRequest{Settings: req}); err != nil {
		return errs.Wrap("m.api.SetMine", fromStatus(err))
	}

	return nil
}

func fromProtoMine(s *userv1.MineState) model.Mine {
	miner := s.GetMiner()

	return model.Mine{
		Settings: fromProtoMineSettings(s.GetSettings()),
		Miner: model.Miner{
			Energy:            int(miner.GetEnergy()),
			NextEnergySeconds: int(miner.GetNextEnergySeconds()),
			Experience:        miner.GetExperience(),
			Backpack:          miner.GetBackpack(),
			Digs:              miner.GetDigs(),
			Earned:            miner.GetEarned(),
		},
	}
}

func fromProtoMineSettings(s *userv1.MineSettings) model.MineSettings {
	out := model.MineSettings{Energy: int(s.GetEnergy()), RegenSeconds: int(s.GetRegenSeconds())}
	for _, v := range s.GetVeins() {
		out.Veins = append(out.Veins, model.OreVein{
			Ore: v.GetOre(), Experience: v.GetExperience(), Least: v.GetLeast(), Most: v.GetMost(),
			Price: v.GetPrice(), Gain: v.GetGain(), Energy: int(v.GetEnergy()),
		})
	}

	e := s.GetEvents()
	out.Events = model.MineEvents{
		Nugget: int(e.GetNugget()), NuggetTimes: int(e.GetNuggetTimes()),
		Chest: int(e.GetChest()), ChestLeast: e.GetChestLeast(), ChestMost: e.GetChestMost(),
		Collapse: int(e.GetCollapse()), Drink: int(e.GetDrink()), DrinkEnergy: int(e.GetDrinkEnergy()),
	}

	return out
}

// int32Of is n, or an error past what the field holds: the user service
// refuses it anyway, with the text of its bounds.
func int32Of(n int, what string) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, failure.New(failure.InvalidArgument, what+" is out of range")
	}

	return int32(n), nil
}

func toProtoMineSettings(s model.MineSettings) (*userv1.MineSettings, error) {
	out := &userv1.MineSettings{Events: &userv1.MineEvents{ChestLeast: s.Events.ChestLeast, ChestMost: s.Events.ChestMost}}

	ints := []struct {
		n    int
		what string
		to   *int32
	}{
		{s.Energy, "the energy", &out.Energy},
		{s.RegenSeconds, "the regeneration", &out.RegenSeconds},
		{s.Events.Nugget, "the chance of a nugget", &out.Events.Nugget},
		{s.Events.NuggetTimes, "the nugget", &out.Events.NuggetTimes},
		{s.Events.Chest, "the chance of a chest", &out.Events.Chest},
		{s.Events.Collapse, "the chance of a collapse", &out.Events.Collapse},
		{s.Events.Drink, "the chance of a drink", &out.Events.Drink},
		{s.Events.DrinkEnergy, "the drink", &out.Events.DrinkEnergy},
	}
	for _, i := range ints {
		n, err := int32Of(i.n, i.what)
		if err != nil {
			return nil, err
		}

		*i.to = n
	}

	for _, v := range s.Veins {
		energy, err := int32Of(v.Energy, "the energy of a dig")
		if err != nil {
			return nil, err
		}

		out.Veins = append(out.Veins, &userv1.OreVein{
			Ore: v.Ore, Experience: v.Experience, Least: v.Least, Most: v.Most,
			Price: v.Price, Gain: v.Gain, Energy: energy,
		})
	}

	return out, nil
}
