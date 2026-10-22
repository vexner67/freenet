package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Mine is the mine of the site, which the user service keeps with the
// votes: people dig ore and sell it, the administrators tune it.
type Mine interface {
	Get(ctx context.Context, accessToken string) (model.Mine, error)
	Dig(ctx context.Context, accessToken, ore string) (model.MineDig, error)
	Sell(ctx context.Context, accessToken, ore string) (model.MineSale, error)
	Settings(ctx context.Context, accessToken string) (model.MineSettings, model.MineStats, error)
	Set(ctx context.Context, accessToken string, settings model.MineSettings) error
}

type oreVeinJSON struct {
	Ore        string `json:"ore"`
	Experience int64  `json:"experience"`
	Least      int64  `json:"least"`
	Most       int64  `json:"most"`
	Price      int64  `json:"price"`
	Gain       int64  `json:"gain"`
	Energy     int    `json:"energy"`
}

type mineEventsJSON struct {
	Nugget      int   `json:"nugget"`
	NuggetTimes int   `json:"nuggetTimes"`
	Chest       int   `json:"chest"`
	ChestLeast  int64 `json:"chestLeast"`
	ChestMost   int64 `json:"chestMost"`
	Collapse    int   `json:"collapse"`
	Drink       int   `json:"drink"`
	DrinkEnergy int   `json:"drinkEnergy"`
}

type mineSettingsJSON struct {
	Veins        []oreVeinJSON  `json:"veins"`
	Energy       int            `json:"energy"`
	RegenSeconds int            `json:"regenSeconds"`
	Events       mineEventsJSON `json:"events"`
}

type minerJSON struct {
	Energy            int              `json:"energy"`
	NextEnergySeconds int              `json:"nextEnergySeconds"`
	Experience        int64            `json:"experience"`
	Backpack          map[string]int64 `json:"backpack"`
	Digs              int64            `json:"digs"`
	Earned            int64            `json:"earned"`
}

type mineJSON struct {
	Settings mineSettingsJSON `json:"settings"`
	Miner    minerJSON        `json:"miner"`
}

func toMineSettingsJSON(s model.MineSettings) mineSettingsJSON {
	out := mineSettingsJSON{
		Veins:        make([]oreVeinJSON, 0, len(s.Veins)),
		Energy:       s.Energy,
		RegenSeconds: s.RegenSeconds,
		Events:       mineEventsJSON(s.Events),
	}
	for _, v := range s.Veins {
		out.Veins = append(out.Veins, oreVeinJSON(v))
	}

	return out
}

func toMineJSON(m model.Mine) mineJSON {
	miner := minerJSON(m.Miner)
	if miner.Backpack == nil {
		miner.Backpack = map[string]int64{}
	}

	return mineJSON{Settings: toMineSettingsJSON(m.Settings), Miner: miner}
}

// mine is "Шахта" of the caller: the ores, the energy and the backpack.
func (a *api) mine(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.mines.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toMineJSON(m))
}

func (a *api) digOre(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Ore string `json:"ore"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	dug, err := a.mines.Dig(r.Context(), token, req.Ore)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Units      int64    `json:"units"`
		Event      string   `json:"event"`
		Votes      int64    `json:"votes"`
		Balance    int64    `json:"balance"`
		Experience int64    `json:"experience"`
		Mine       mineJSON `json:"mine"`
	}{dug.Units, dug.Event, dug.Votes, dug.Balance, dug.Experience, toMineJSON(dug.Mine)})
}

// sellOre sells all of an ore, or of every ore without one.
func (a *api) sellOre(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Ore string `json:"ore"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	sold, err := a.mines.Sell(r.Context(), token, req.Ore)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Units   int64    `json:"units"`
		Votes   int64    `json:"votes"`
		Balance int64    `json:"balance"`
		Mine    mineJSON `json:"mine"`
	}{sold.Units, sold.Votes, sold.Balance, toMineJSON(sold.Mine)})
}

// mineSettings is "Шахта" of the admin panel: the settings and how the
// mine does.
func (a *api) mineSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, stats, err := a.mines.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Settings mineSettingsJSON `json:"settings"`
		Digs     int64            `json:"digs"`
		Sold     int64            `json:"sold"`
		Chests   int64            `json:"chests"`
		Miners   int64            `json:"miners"`
	}{toMineSettingsJSON(s), stats.Digs, stats.Sold, stats.Chests, stats.Miners})
}

func (a *api) setMine(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req mineSettingsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	settings := model.MineSettings{Energy: req.Energy, RegenSeconds: req.RegenSeconds, Events: model.MineEvents(req.Events)}
	for _, v := range req.Veins {
		settings.Veins = append(settings.Veins, model.OreVein(v))
	}

	if err = a.mines.Set(r.Context(), token, settings); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
