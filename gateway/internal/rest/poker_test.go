package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakePoker struct {
	token string
	ante  int64
	id    string
	call  bool
	set   model.Poker
	dealt bool
}

var fakeHand = model.PokerHand{
	ID: "h1", Ante: 10, State: "dealt", Player: []string{"As", "Kd"}, Board: []string{"Qh", "Jc", "2s"},
	PlayerHand: "high_card", PlayerBest: []string{"As", "Kd", "Qh", "Jc", "2s"}, Balance: 90,
}

func (f *fakePoker) Get(_ context.Context, token string) (model.PokerTable, error) {
	f.token = token

	table := model.PokerTable{Poker: model.Poker{Least: 1, Most: 100, Pays: []int{100, 20, 10, 3, 2, 1}}}
	if f.dealt {
		hand := fakeHand
		table.Dealt = &hand
	}

	return table, nil
}

func (f *fakePoker) Deal(_ context.Context, token string, ante int64) (model.PokerHand, error) {
	f.token, f.ante = token, ante
	return fakeHand, nil
}

func (f *fakePoker) Play(_ context.Context, token, id string, call bool) (model.PokerHand, error) {
	f.token, f.id, f.call = token, id, call
	hand := fakeHand
	hand.State, hand.Dealer, hand.DealerHand, hand.Payout = "won", []string{"3c", "4d"}, "pair", 60

	return hand, nil
}

func (f *fakePoker) Settings(_ context.Context, token string) (model.Poker, model.PokerStats, error) {
	f.token = token
	return model.Poker{Least: 1, Most: 100, Pays: []int{100, 20, 10, 3, 2, 1}}, model.PokerStats{Hands: 3, Spent: 60, Won: 40}, nil
}

func (f *fakePoker) Set(_ context.Context, token string, poker model.Poker) error {
	f.token, f.set = token, poker
	return nil
}

func TestPoker(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/casino/poker", "")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || got["dealt"] != nil || got["poker"].(map[string]any)["most"] != 100.0 || h.poker.token != "" {
		t.Fatalf("poker of a guest: %d %v", w.Code, got)
	}

	h.poker.dealt = true
	w = h.do("GET", "/api/v1/casino/poker", "", "Authorization", "Bearer tok")
	got = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.poker.token != "tok" || got["dealt"].(map[string]any)["id"] != "h1" {
		t.Fatalf("poker: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/casino/poker/hands", `{"ante":10}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("deal by a guest: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/casino/poker/hands", `{"ante":10}`, "Authorization", "Bearer tok")
	hand := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.poker.ante != 10 || hand["balance"] != 90.0 || len(hand["dealer"].([]any)) != 0 || len(hand["board"].([]any)) != 3 {
		t.Fatalf("deal: %d %v", w.Code, hand)
	}

	w = h.do("POST", "/api/v1/casino/poker/hands/h1/call", "", "Authorization", "Bearer tok")
	hand = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.poker.id != "h1" || !h.poker.call || hand["state"] != "won" || hand["dealerHand"] != "pair" {
		t.Fatalf("call: %d %v", w.Code, hand)
	}
	if w = h.do("POST", "/api/v1/casino/poker/hands/h1/fold", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.poker.call {
		t.Fatalf("fold: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/casino/poker/hands/h1/raise", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown action: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/casino/poker/settings", "", "Authorization", "Bearer tok")
	if s := decode[map[string]any](t, w); w.Code != http.StatusOK || s["hands"] != 3.0 || len(s["poker"].(map[string]any)["pays"].([]any)) != 6 {
		t.Fatalf("settings: %d %v", w.Code, s)
	}

	w = h.do("PUT", "/api/v1/casino/poker/settings", `{"least":2,"most":20,"pays":[50,10,5,2,2,1]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.poker.set.Most != 20 || len(h.poker.set.Pays) != 6 {
		t.Fatalf("set settings: %d %+v", w.Code, h.poker.set)
	}
}
