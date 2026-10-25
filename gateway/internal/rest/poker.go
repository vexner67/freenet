package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Poker is "Покер" of the casino: Casino Hold'em against the dealer, which
// the user service keeps with the votes.
type Poker interface {
	Get(ctx context.Context, accessToken string) (model.PokerTable, error)
	Deal(ctx context.Context, accessToken string, ante int64) (model.PokerHand, error)
	Play(ctx context.Context, accessToken, id string, call bool) (model.PokerHand, error)
	Settings(ctx context.Context, accessToken string) (model.Poker, model.PokerStats, error)
	Set(ctx context.Context, accessToken string, poker model.Poker) error
}

type pokerJSON struct {
	Least int64 `json:"least"`
	Most  int64 `json:"most"`
	// Pays are the pays of the ante to one: straight, flush, full house,
	// four of a kind, straight flush, royal flush.
	Pays []int `json:"pays"`
}

// pokerHandJSON is a hand of "Покер": the dealer and his hand only after
// the call.
type pokerHandJSON struct {
	ID         string   `json:"id"`
	Ante       int64    `json:"ante"`
	State      string   `json:"state"`
	Player     []string `json:"player"`
	Board      []string `json:"board"`
	Dealer     []string `json:"dealer"`
	Payout     int64    `json:"payout"`
	PlayerHand string   `json:"playerHand"`
	DealerHand string   `json:"dealerHand"`
	PlayerBest []string `json:"playerBest"`
	DealerBest []string `json:"dealerBest"`
	Balance    int64    `json:"balance"`
}

func toPokerJSON(p model.Poker) pokerJSON {
	pays := p.Pays
	if pays == nil {
		pays = []int{}
	}

	return pokerJSON{Least: p.Least, Most: p.Most, Pays: pays}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}

	return s
}

func toPokerHandJSON(h model.PokerHand) pokerHandJSON {
	return pokerHandJSON{
		ID:         h.ID,
		Ante:       h.Ante,
		State:      h.State,
		Player:     nonNil(h.Player),
		Board:      nonNil(h.Board),
		Dealer:     nonNil(h.Dealer),
		Payout:     h.Payout,
		PlayerHand: h.PlayerHand,
		DealerHand: h.DealerHand,
		PlayerBest: nonNil(h.PlayerBest),
		DealerBest: nonNil(h.DealerBest),
		Balance:    h.Balance,
	}
}

// poker is the table and, for a signed-in viewer, the hand waiting.
func (a *api) poker(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	table, err := a.pokers.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := struct {
		Poker pokerJSON      `json:"poker"`
		Dealt *pokerHandJSON `json:"dealt"`
	}{Poker: toPokerJSON(table.Poker)}
	if table.Dealt != nil {
		hand := toPokerHandJSON(*table.Dealt)
		out.Dealt = &hand
	}

	writeJSON(w, http.StatusOK, out)
}

func (a *api) dealPoker(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Ante int64 `json:"ante"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	hand, err := a.pokers.Deal(r.Context(), token, req.Ante)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPokerHandJSON(hand))
}

// playPoker calls or folds a hand of the caller.
func (a *api) playPoker(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	action := r.PathValue("action")
	if action != "call" && action != "fold" {
		a.writeError(w, r, failure.New(failure.NotFound, "unknown action of poker"))
		return
	}

	hand, err := a.pokers.Play(r.Context(), token, r.PathValue("id"), action == "call")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPokerHandJSON(hand))
}

// pokerSettings is "Покер" of the admin panel: the table and how it does.
func (a *api) pokerSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	poker, stats, err := a.pokers.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Poker pokerJSON `json:"poker"`
		Hands int64     `json:"hands"`
		Spent int64     `json:"spent"`
		Won   int64     `json:"won"`
	}{toPokerJSON(poker), stats.Hands, stats.Spent, stats.Won})
}

func (a *api) setPokerSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req pokerJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.pokers.Set(r.Context(), token, model.Poker(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
