package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeHoldem struct {
	token   string
	section string
	action  string
	amount  int64
	seat    int
	closed  bool
	invited []string
}

func fakeTable() model.HoldemTable {
	return model.HoldemTable{
		ID: "t1", Name: "Пятница", OwnerID: "u1", BigBlind: 10, SmallBlind: 5, Street: "flop", Turn: 1, TurnMs: 20000,
		Board: []string{"As", "Kd", "2c"}, Pot: 40, Version: 7, You: 0, Players: 2,
		Seats: []model.HoldemSeat{
			{UserID: "u1", Stack: 980, Cards: []string{"Ah", "Ac"}, HasCards: true, Hand: "three"},
			{UserID: "u2", Stack: 980, HasCards: true},
			{},
		},
	}
}

func (f *fakeHoldem) List(_ context.Context, token, section string) ([]model.HoldemTable, error) {
	f.token, f.section = token, section
	return []model.HoldemTable{fakeTable()}, nil
}

func (f *fakeHoldem) Create(_ context.Context, token string, _ model.NewHoldemTable) (*model.HoldemTable, int64, error) {
	f.token = token
	t := fakeTable()
	return &t, 800, nil
}

func (f *fakeHoldem) Quick(_ context.Context, token string, _, _ int64) (*model.HoldemTable, int64, error) {
	f.token = token
	t := fakeTable()
	return &t, 800, nil
}

func (f *fakeHoldem) Get(_ context.Context, token, _ string) (model.HoldemTable, error) {
	f.token = token
	return fakeTable(), nil
}

func (f *fakeHoldem) Sit(_ context.Context, token, _ string, seat int, _ int64) (*model.HoldemTable, int64, error) {
	f.token, f.seat = token, seat
	t := fakeTable()
	return &t, 800, nil
}

func (f *fakeHoldem) Leave(_ context.Context, token, _ string) (*model.HoldemTable, int64, error) {
	f.token = token
	if f.closed {
		return nil, 1000, nil
	}
	t := fakeTable()
	return &t, 1000, nil
}

func (f *fakeHoldem) Act(_ context.Context, token, _, action string, amount int64) (model.HoldemTable, error) {
	f.token, f.action, f.amount = token, action, amount
	return fakeTable(), nil
}

func (f *fakeHoldem) Back(_ context.Context, token, _ string) (model.HoldemTable, error) {
	f.token = token
	return fakeTable(), nil
}

func (f *fakeHoldem) Invite(_ context.Context, token, _ string, nicknames []string) (model.HoldemTable, error) {
	f.token, f.invited = token, nicknames
	return fakeTable(), nil
}

func TestHoldem(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/casino/tables?section=public", "")
	got := decode[map[string]any](t, w)
	tables, _ := got["tables"].([]any)
	if w.Code != http.StatusOK || len(tables) != 1 || h.holdem.section != "public" {
		t.Fatalf("tables: %d %v", w.Code, got)
	}
	table := tables[0].(map[string]any)
	seats := table["seats"].([]any)
	if seats[1].(map[string]any)["player"].(map[string]any)["nickname"] != "bob" || seats[2].(map[string]any)["player"] != nil {
		t.Fatalf("seats: %v", seats)
	}

	w = h.do("GET", "/api/v1/casino/tables/t1?version=7", "")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["same"] != true {
		t.Fatalf("the same version: %d %v", w.Code, got)
	}
	w = h.do("GET", "/api/v1/casino/tables/t1?version=6", "")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["pot"] != 40.0 {
		t.Fatalf("a new version: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/casino/tables/quick", `{"bigBlind":10,"buyIn":200}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("quick by a guest: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/casino/tables/quick", `{"bigBlind":10,"buyIn":200}`, "Authorization", "Bearer tok")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["balance"] != 800.0 || h.holdem.token != "tok" {
		t.Fatalf("quick: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/casino/tables/t1/seat", `{"buyIn":200}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.holdem.seat != -1 {
		t.Fatalf("sit: %d, seat %d", w.Code, h.holdem.seat)
	}

	if w = h.do("POST", "/api/v1/casino/tables/t1/actions", `{"action":"dance"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown action: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/casino/tables/t1/actions", `{"action":"raise","amount":60}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.holdem.action != "raise" || h.holdem.amount != 60 {
		t.Fatalf("act: %d %+v", w.Code, h.holdem)
	}

	w = h.do("POST", "/api/v1/casino/tables/t1/invitations", `{"nicknames":["bob"]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || len(h.holdem.invited) != 1 {
		t.Fatalf("invite: %d", w.Code)
	}

	h.holdem.closed = true
	w = h.do("DELETE", "/api/v1/casino/tables/t1/seat", "", "Authorization", "Bearer tok")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["table"] != nil || got["balance"] != 1000.0 {
		t.Fatalf("leave: %d %v", w.Code, got)
	}
}
