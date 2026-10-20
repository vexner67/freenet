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
	token string
	set   model.Casino
	err   error
}

func (f *fakeCasino) Get(context.Context) (model.Casino, error) {
	return model.Casino{Price: 10, Prizes: []model.CasinoPrize{{Votes: 0}, {Votes: 50}}}, nil
}

func (f *fakeCasino) Settings(_ context.Context, token string) (model.CasinoSettings, error) {
	if f.err != nil {
		return model.CasinoSettings{}, f.err
	}
	f.token = token

	return model.CasinoSettings{
		Casino: model.Casino{Price: 10, Prizes: []model.CasinoPrize{{Votes: 0, Weight: 9}, {Votes: 50, Weight: 1}}},
		Spins:  2, Spent: 20, Won: 50,
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
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["price"] != 10.0 || len(got["prizes"].([]any)) != 2 {
		t.Fatalf("%d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/casino/settings", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if prizes := got["prizes"].([]any); w.Code != http.StatusOK || prizes[0].(map[string]any)["weight"] != 9.0 || got["won"] != 50.0 || h.casino.token != "tok" {
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

	for _, path := range []string{"/api/v1/casino/spins"} {
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
