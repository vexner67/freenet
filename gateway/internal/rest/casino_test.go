package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeCasino struct {
	token   string
	set     model.Casino
	bets    model.CasinoBets
	amount  int64
	times   int
	rocket  model.CasinoRocket
	id      string
	jackpot int64
	err     error
}

func (f *fakeCasino) SetRocket(_ context.Context, token string, rocket model.CasinoRocket) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.rocket = token, rocket

	return nil
}

func (f *fakeCasino) SetJackpot(_ context.Context, token string, jackpot int64) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.jackpot = token, jackpot

	return nil
}

func (f *fakeCasino) LaunchRocket(_ context.Context, token string, amount int64, auto int) (model.RocketRound, error) {
	if f.err != nil {
		return model.RocketRound{}, f.err
	}
	f.token, f.amount, f.times = token, amount, auto

	return model.RocketRound{ID: "r1", Amount: amount, Auto: auto, State: model.RocketFlying, ElapsedMS: 2, Balance: 90}, nil
}

func (f *fakeCasino) Rocket(_ context.Context, token, id string) (model.RocketRound, error) {
	if f.err != nil {
		return model.RocketRound{}, f.err
	}
	f.token, f.id = token, id

	return model.RocketRound{ID: id, Amount: 10, State: model.RocketCrashed, Crash: 237, ElapsedMS: 9000}, nil
}

func (f *fakeCasino) CashRocket(_ context.Context, token, id string) (model.RocketRound, error) {
	if f.err != nil {
		return model.RocketRound{}, f.err
	}
	f.token, f.id = token, id

	return model.RocketRound{ID: id, Amount: 10, State: model.RocketCashed, Cashed: 200, Payout: 20, Crash: 237, Balance: 110}, nil
}

func (f *fakeCasino) Get(context.Context) (model.Casino, error) {
	return model.Casino{
		Price:   10,
		Prizes:  []model.CasinoPrize{{Votes: 0}, {Votes: 50}},
		Bets:    model.CasinoBets{Least: 1, Most: 100, Multipliers: []model.AimMultiplier{{Multiplier: 150, Chance: 6333}, {Multiplier: 200, Chance: 4750}}},
		Rocket:  model.CasinoRocket{Least: 1, Most: 1000, Return: 9500, Top: 100000},
		Jackpot: 1000,
	}, nil
}

func (f *fakeCasino) SetBets(_ context.Context, token string, bets model.CasinoBets) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.bets = token, bets

	return nil
}

func (f *fakeCasino) Bet(_ context.Context, token string, amount int64, multiplier int) (model.CasinoWager, error) {
	if f.err != nil {
		return model.CasinoWager{}, f.err
	}
	f.token, f.amount, f.times = token, amount, multiplier

	return model.CasinoWager{Roll: 6000, Chance: 4750, Balance: 5}, nil
}

func (f *fakeCasino) Settings(_ context.Context, token string) (model.CasinoSettings, error) {
	if f.err != nil {
		return model.CasinoSettings{}, f.err
	}
	f.token = token

	return model.CasinoSettings{
		Casino: model.Casino{
			Price:   10,
			Prizes:  []model.CasinoPrize{{Votes: 0, Weight: 9}, {Votes: 50, Weight: 1}},
			Bets:    model.CasinoBets{Least: 1, Most: 100, Multipliers: []model.AimMultiplier{{Multiplier: 150, Chance: 6333}, {Multiplier: 200, Chance: 4750}}},
			Rocket:  model.CasinoRocket{Least: 1, Most: 1000, Return: 9500, Top: 100000},
			Jackpot: 700,
		},
		Spins: 2, Spent: 20, Won: 50,
		Wagers: 3, WagerSpent: 30, WagerWon: 20,
		Rockets: 4, RocketSpent: 40, RocketWon: 38,
	}, nil
}

func (f *fakeCasino) Set(_ context.Context, token string, casino model.Casino) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.set = token, casino

	return nil
}

func (f *fakeCasino) Spin(_ context.Context, token string) (model.CasinoSpin, error) {
	if f.err != nil {
		return model.CasinoSpin{}, f.err
	}
	f.token = token

	return model.CasinoSpin{Prize: 1, Votes: 50, Balance: 55}, nil
}

func TestCasino(t *testing.T) {
	t.Parallel()

	// A guest sees the price and the prizes, never the odds.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/casino", "")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["price"] != 10.0 || len(got["prizes"].([]any)) != 2 ||
		got["bets"].(map[string]any)["most"] != 100.0 || len(got["bets"].(map[string]any)["multipliers"].([]any)) != 2 {
		t.Fatalf("%d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/casino/settings", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if prizes := got["prizes"].([]any); w.Code != http.StatusOK || prizes[0].(map[string]any)["weight"] != 9.0 || got["won"] != 50.0 || h.casino.token != "tok" ||
		got["wagers"].(map[string]any)["count"] != 3.0 || got["wagers"].(map[string]any)["won"] != 20.0 ||
		len(got["bets"].(map[string]any)["multipliers"].([]any)) != 2 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/casino", `{"price":5,"prizes":[{"votes":0,"weight":3},{"votes":20,"weight":1}]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.casino.set.Price != 5 || h.casino.set.Prizes[1] != (model.CasinoPrize{Votes: 20, Weight: 1}) {
		t.Fatalf("set: %d %+v", w.Code, h.casino.set)
	}

	w = h.do("POST", "/api/v1/casino/spins", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["prize"] != 1.0 || got["votes"] != 50.0 || got["balance"] != 55.0 {
		t.Fatalf("spin: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/casino/bets", `{"least":2,"most":20,"multipliers":[{"multiplier":250,"chance":3800}]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.casino.bets.Least != 2 || h.casino.bets.Most != 20 || h.casino.bets.Multipliers[0] != (model.AimMultiplier{Multiplier: 250, Chance: 3800}) {
		t.Fatalf("set bets: %d %+v", w.Code, h.casino.bets)
	}

	w = h.do("POST", "/api/v1/casino/wagers", `{"amount":10,"multiplier":200}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["won"] != false || got["roll"] != 6000.0 || got["chance"] != 4750.0 ||
		got["balance"] != 5.0 || h.casino.amount != 10 || h.casino.times != 200 {
		t.Fatalf("bet: %d %v", w.Code, got)
	}

	for _, path := range []string{"/api/v1/casino/spins", "/api/v1/casino/wagers"} {
		if w = newHarness(rest.Config{}).do("POST", path, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token: %d", path, w.Code)
		}
	}

	h = newHarness(rest.Config{})
	h.casino.err = failure.New(failure.Conflict, "not enough votes")
	if w = h.do("POST", "/api/v1/casino/spins", "", "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("without votes: %d", w.Code)
	}

	h.casino.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("PUT", "/api/v1/casino", `{"price":5,"prizes":[]}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("not an administrator: %d", w.Code)
	}
}

func TestCasino_Rocket(t *testing.T) {
	t.Parallel()

	// A guest sees the limits and the top, never the return.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/casino", "")
	casino := decode[map[string]any](t, w)
	rocket := casino["rocket"].(map[string]any)
	if w.Code != http.StatusOK || casino["jackpot"] != 1000.0 || rocket["most"] != 1000.0 || rocket["top"] != 100000.0 || rocket["return"] != nil {
		t.Fatalf("%d %v", w.Code, rocket)
	}

	w = h.do("GET", "/api/v1/casino/settings", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || got["jackpot"] != 700.0 || got["rocket"].(map[string]any)["return"] != 9500.0 || got["rockets"].(map[string]any)["won"] != 38.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/casino/rocket", `{"least":2,"most":20,"return":9000,"top":5000}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.casino.rocket != (model.CasinoRocket{Least: 2, Most: 20, Return: 9000, Top: 5000}) {
		t.Fatalf("set rocket: %d %+v", w.Code, h.casino.rocket)
	}

	w = h.do("PUT", "/api/v1/casino/jackpot", `{"jackpot":500}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.casino.jackpot != 500 {
		t.Fatalf("set jackpot: %d %d", w.Code, h.casino.jackpot)
	}

	w = h.do("POST", "/api/v1/casino/rockets", `{"amount":10,"auto":250}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["id"] != "r1" || got["state"] != "flying" || got["auto"] != 250.0 ||
		got["crash"] != 0.0 || got["elapsedMs"] != 2.0 || got["balance"] != 90.0 || h.casino.amount != 10 || h.casino.times != 250 {
		t.Fatalf("launch: %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/casino/rockets/r1", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["state"] != "crashed" || got["crash"] != 237.0 || h.casino.id != "r1" {
		t.Fatalf("rocket: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/casino/rockets/r2/cash", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["state"] != "cashed" || got["cashed"] != 200.0 || got["payout"] != 20.0 ||
		got["balance"] != 110.0 || h.casino.id != "r2" {
		t.Fatalf("cash: %d %v", w.Code, got)
	}

	for _, path := range []string{"/api/v1/casino/rockets", "/api/v1/casino/rockets/r1/cash"} {
		if w = newHarness(rest.Config{}).do("POST", path, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token: %d", path, w.Code)
		}
	}

	h.casino.err = failure.New(failure.NotFound, "rocket not found")
	if w = h.do("GET", "/api/v1/casino/rockets/r9", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's rocket: %d", w.Code)
	}
}
