package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeBets struct {
	token   string
	popular bool
	draft   model.BetEventDraft
	// done are the changes made: "close <id>", "resolve <id>"…
	done    []string
	winner  int
	outcome int
	amount  int64
	err     error
}

func (f *fakeBets) List(_ context.Context, token string, popular bool) ([]model.BetEvent, error) {
	f.token, f.popular = token, popular

	return []model.BetEvent{{
		ID:        "e1",
		Question:  "Проиграют ли?",
		Outcomes:  []model.BetOutcome{{Name: "Да", Staked: 30, Bettors: 2, Mine: 10}, {Name: "Нет", Staked: 70, Bettors: 1}},
		State:     "open",
		Open:      true,
		Winner:    -1,
		CreatedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		People:    3,
	}}, nil
}

func (f *fakeBets) Create(_ context.Context, token string, draft model.BetEventDraft) (model.BetEvent, error) {
	if f.err != nil {
		return model.BetEvent{}, f.err
	}
	f.token, f.draft = token, draft

	return model.BetEvent{ID: "e2", Question: draft.Question, State: "open", Open: true, Winner: -1, ClosesAt: draft.ClosesAt}, nil
}

func (f *fakeBets) change(token, what, id string) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.done = token, append(f.done, what+" "+id)

	return nil
}

func (f *fakeBets) Close(_ context.Context, token, id string) error {
	return f.change(token, "close", id)
}

func (f *fakeBets) Cancel(_ context.Context, token, id string) error {
	return f.change(token, "cancel", id)
}

func (f *fakeBets) Delete(_ context.Context, token, id string) error {
	return f.change(token, "delete", id)
}

func (f *fakeBets) Resolve(_ context.Context, token, id string, winner int) error {
	f.winner = winner

	return f.change(token, "resolve", id)
}

func (f *fakeBets) Stake(_ context.Context, token, _ string, outcome int, amount int64) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.token, f.outcome, f.amount = token, outcome, amount

	return 42, nil
}

func TestBets(t *testing.T) {
	t.Parallel()

	// A guest sees the events.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/bets?section=popular", "")
	got := decode[map[string]any](t, w)
	events := got["events"].([]any)
	e := events[0].(map[string]any)
	if w.Code != http.StatusOK || !h.bets.popular || h.bets.token != "" || e["staked"] != 100.0 || e["closesAt"] != nil ||
		e["outcomes"].([]any)[0].(map[string]any)["mine"] != 10.0 || e["people"] != 3.0 || e["createdAt"] != "2026-10-20T00:00:00Z" {
		t.Fatalf("list: %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/bets", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.bets.popular || h.bets.token != "tok" {
		t.Fatalf("list with a token: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/bets", `{"question":"Q?","outcomes":["Да","Нет"],"closesAt":"2026-11-03T18:00:00Z"}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusCreated || got["closesAt"] != "2026-11-03T18:00:00Z" ||
		len(h.bets.draft.Outcomes) != 2 || !h.bets.draft.ClosesAt.Equal(time.Date(2026, 11, 3, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("create: %d %v", w.Code, got)
	}
	if w = h.do("POST", "/api/v1/bets", `{"question":"Q?","outcomes":["Да","Нет"],"closesAt":"tomorrow"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("create with a bad time: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/bets/e1/stakes", `{"outcome":1,"amount":25}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["balance"] != 42.0 || h.bets.outcome != 1 || h.bets.amount != 25 {
		t.Fatalf("stake: %d %v", w.Code, got)
	}

	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/v1/bets/e1/close", ""},
		{"POST", "/api/v1/bets/e1/resolve", `{"winner":1}`},
		{"POST", "/api/v1/bets/e1/cancel", ""},
		{"DELETE", "/api/v1/bets/e1", ""},
	} {
		if w = h.do(c.method, c.path, c.body, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
			t.Fatalf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
	if want := []string{"close e1", "resolve e1", "cancel e1", "delete e1"}; len(h.bets.done) != 4 || h.bets.done[1] != want[1] || h.bets.done[3] != want[3] || h.bets.winner != 1 {
		t.Fatalf("done = %v, winner %d", h.bets.done, h.bets.winner)
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/bets/e1/stakes", `{"outcome":0,"amount":1}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("stake without token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	h.bets.err = failure.New(failure.Conflict, "the event takes no more stakes")
	if w = h.do("POST", "/api/v1/bets/e1/stakes", `{"outcome":0,"amount":1}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("stake on a closed event: %d", w.Code)
	}
	h.bets.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("POST", "/api/v1/bets/e1/close", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("not an administrator: %d", w.Code)
	}
}
