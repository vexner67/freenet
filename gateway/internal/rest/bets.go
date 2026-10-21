package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Bets are the bets on events, which the user service keeps with the
// votes: the administrators ask, people stake.
type Bets interface {
	List(ctx context.Context, accessToken string, popular bool) ([]model.BetEvent, error)
	Create(ctx context.Context, accessToken string, draft model.BetEventDraft) (model.BetEvent, error)
	Close(ctx context.Context, accessToken, id string) error
	Resolve(ctx context.Context, accessToken, id string, winner int) error
	Cancel(ctx context.Context, accessToken, id string) error
	Delete(ctx context.Context, accessToken, id string) error
	SetOdds(ctx context.Context, accessToken, id string, odds []int) error
	Stake(ctx context.Context, accessToken, id string, outcome int, amount int64) (int64, error)
}

type betOutcomeJSON struct {
	Name string `json:"name"`
	// Odds are in hundredths (250 — ×2.5), 0 for an event without odds.
	Odds    int   `json:"odds"`
	Staked  int64 `json:"staked"`
	Bettors int   `json:"bettors"`
	Mine    int64 `json:"mine"`
	// Potential is what the stakes of the viewer pay if it wins.
	Potential int64 `json:"potential"`
}

type betEventJSON struct {
	ID       string           `json:"id"`
	Question string           `json:"question"`
	Outcomes []betOutcomeJSON `json:"outcomes"`
	State    string           `json:"state"`
	Open     bool             `json:"open"`
	// ClosesAt is RFC 3339, null for never.
	ClosesAt  *string `json:"closesAt"`
	Winner    int     `json:"winner"`
	CreatedAt string  `json:"createdAt"`
	// Staked is the votes on all the outcomes.
	Staked int64 `json:"staked"`
	People int   `json:"people"`
	Payout int64 `json:"payout"`
}

func toBetEventJSON(e model.BetEvent) betEventJSON {
	out := betEventJSON{
		ID:        e.ID,
		Question:  e.Question,
		Outcomes:  make([]betOutcomeJSON, 0, len(e.Outcomes)),
		State:     e.State,
		Open:      e.Open,
		Winner:    e.Winner,
		CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		People:    e.People,
		Payout:    e.Payout,
	}
	if !e.ClosesAt.IsZero() {
		closesAt := e.ClosesAt.UTC().Format(time.RFC3339)
		out.ClosesAt = &closesAt
	}

	for _, o := range e.Outcomes {
		out.Outcomes = append(out.Outcomes, betOutcomeJSON(o))
		out.Staked += o.Staked
	}

	return out
}

// betEvents is public: "Ставки" of the casino, ?section=popular for the
// events most staked on.
func (a *api) betEvents(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	events, err := a.bets.List(r.Context(), token, r.URL.Query().Get("section") == "popular")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := make([]betEventJSON, 0, len(events))
	for _, e := range events {
		out = append(out, toBetEventJSON(e))
	}

	writeJSON(w, http.StatusOK, struct {
		Events []betEventJSON `json:"events"`
	}{out})
}

// createBetEvent is for the administrators.
func (a *api) createBetEvent(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Question string   `json:"question"`
		Outcomes []string `json:"outcomes"`
		Odds     []int    `json:"odds"`
		ClosesAt string   `json:"closesAt"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	draft := model.BetEventDraft{Question: req.Question, Outcomes: req.Outcomes, Odds: req.Odds}
	if req.ClosesAt != "" {
		if draft.ClosesAt, err = time.Parse(time.RFC3339, req.ClosesAt); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid closing time"))
			return
		}
	}

	e, err := a.bets.Create(r.Context(), token, draft)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toBetEventJSON(e))
}

// changeBetEvent is an administrator's close, cancel or delete of an
// event.
func (a *api) changeBetEvent(change func(b Bets, ctx context.Context, accessToken, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerToken(r)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		if err = change(a.bets, r.Context(), token, r.PathValue("id")); err != nil {
			a.writeError(w, r, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *api) resolveBetEvent(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Winner int `json:"winner"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.bets.Resolve(r.Context(), token, r.PathValue("id"), req.Winner); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// setBetEventOdds is for the administrators: the new odds of each
// outcome, for the stakes from now on.
func (a *api) setBetEventOdds(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Odds []int `json:"odds"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.bets.SetOdds(r.Context(), token, r.PathValue("id"), req.Odds); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// stakeBet puts votes on an outcome of an event.
func (a *api) stakeBet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Outcome int   `json:"outcome"`
		Amount  int64 `json:"amount"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	balance, err := a.bets.Stake(r.Context(), token, r.PathValue("id"), req.Outcome, req.Amount)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Balance int64 `json:"balance"`
	}{balance})
}
