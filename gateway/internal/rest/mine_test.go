package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeMine struct {
	token string
	ore   string
	set   model.MineSettings
	err   error
}

func (f *fakeMine) settings() model.MineSettings {
	return model.MineSettings{
		Veins:        []model.OreVein{{Ore: "stone", Least: 120, Most: 180, Price: 2, Gain: 10, Energy: 1}, {Ore: "coal", Experience: 500, Energy: 1}},
		Energy:       10,
		RegenSeconds: 900,
		Events:       model.MineEvents{Nugget: 300, NuggetTimes: 3},
	}
}

func (f *fakeMine) mine() model.Mine {
	return model.Mine{
		Settings: f.settings(),
		Miner:    model.Miner{Energy: 9, NextEnergySeconds: 600, Experience: 10, Backpack: map[string]int64{"stone": 150}, Digs: 1},
	}
}

func (f *fakeMine) Get(_ context.Context, token string) (model.Mine, error) {
	if f.err != nil {
		return model.Mine{}, f.err
	}
	f.token = token

	return f.mine(), nil
}

func (f *fakeMine) Dig(_ context.Context, token, ore string) (model.MineDig, error) {
	if f.err != nil {
		return model.MineDig{}, f.err
	}
	f.token, f.ore = token, ore

	return model.MineDig{Units: 150, Event: "chest", Votes: 30, Balance: 130, Experience: 10, Mine: f.mine()}, nil
}

func (f *fakeMine) Sell(_ context.Context, token, ore string) (model.MineSale, error) {
	if f.err != nil {
		return model.MineSale{}, f.err
	}
	f.token, f.ore = token, ore

	return model.MineSale{Units: 150, Votes: 3, Balance: 133, Mine: f.mine()}, nil
}

func (f *fakeMine) Settings(_ context.Context, token string) (model.MineSettings, model.MineStats, error) {
	if f.err != nil {
		return model.MineSettings{}, model.MineStats{}, f.err
	}
	f.token = token

	return f.settings(), model.MineStats{Digs: 7, Sold: 40, Chests: 30, Miners: 2}, nil
}

func (f *fakeMine) Set(_ context.Context, token string, settings model.MineSettings) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.set = token, settings

	return nil
}

func TestMine(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/me/mine", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/me/mine", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	miner := got["miner"].(map[string]any)
	settings := got["settings"].(map[string]any)
	if w.Code != http.StatusOK || h.mine.token != "tok" || miner["nextEnergySeconds"] != 600.0 ||
		miner["backpack"].(map[string]any)["stone"] != 150.0 || settings["regenSeconds"] != 900.0 ||
		len(settings["veins"].([]any)) != 2 || settings["events"].(map[string]any)["nuggetTimes"] != 3.0 {
		t.Fatalf("mine: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/mine/digs", `{"ore":"stone"}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || h.mine.ore != "stone" || got["units"] != 150.0 ||
		got["event"] != "chest" || got["balance"] != 130.0 || got["mine"] == nil {
		t.Fatalf("dig: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/mine/sales", `{"ore":""}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || h.mine.ore != "" || got["votes"] != 3.0 || got["balance"] != 133.0 {
		t.Fatalf("sell: %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/mine/settings", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["digs"] != 7.0 || got["miners"] != 2.0 ||
		got["settings"].(map[string]any)["energy"] != 10.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}

	body := `{"veins":[{"ore":"stone","experience":0,"least":1,"most":2,"price":3,"gain":4,"energy":1}],"energy":20,"regenSeconds":60,` +
		`"events":{"nugget":1,"nuggetTimes":2,"chest":3,"chestLeast":4,"chestMost":5,"collapse":6,"drink":7,"drinkEnergy":8}}`
	w = h.do("PUT", "/api/v1/mine/settings", body, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.mine.set.Energy != 20 || h.mine.set.RegenSeconds != 60 ||
		h.mine.set.Veins[0] != (model.OreVein{Ore: "stone", Least: 1, Most: 2, Price: 3, Gain: 4, Energy: 1}) ||
		h.mine.set.Events.DrinkEnergy != 8 {
		t.Fatalf("set: %d %+v", w.Code, h.mine.set)
	}

	h.mine.err = failure.New(failure.Conflict, "not enough energy")
	if w = h.do("POST", "/api/v1/me/mine/digs", `{"ore":"stone"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("no energy: %d", w.Code)
	}
}
