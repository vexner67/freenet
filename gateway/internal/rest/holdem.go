package rest

import (
	"context"
	"net/http"
	"strconv"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Holdem are the poker tables of people: no-limit Texas Hold'em, which the
// user service keeps with the votes.
type Holdem interface {
	List(ctx context.Context, accessToken, section string) ([]model.HoldemTable, error)
	Create(ctx context.Context, accessToken string, in model.NewHoldemTable) (*model.HoldemTable, int64, error)
	Quick(ctx context.Context, accessToken string, bigBlind, buyIn int64) (*model.HoldemTable, int64, error)
	Get(ctx context.Context, accessToken, id string) (model.HoldemTable, error)
	Sit(ctx context.Context, accessToken, id string, seat int, buyIn int64) (*model.HoldemTable, int64, error)
	Leave(ctx context.Context, accessToken, id string) (*model.HoldemTable, int64, error)
	Act(ctx context.Context, accessToken, id, action string, amount int64) (model.HoldemTable, error)
	Back(ctx context.Context, accessToken, id string) (model.HoldemTable, error)
	Invite(ctx context.Context, accessToken, id string, nicknames []string) (model.HoldemTable, error)
}

// holdemSeatJSON is a seat; player is null on a free one.
type holdemSeatJSON struct {
	Player   *authorJSON `json:"player"`
	Stack    int64       `json:"stack"`
	Bet      int64       `json:"bet"`
	Cards    []string    `json:"cards"`
	HasCards bool        `json:"hasCards"`
	Folded   bool        `json:"folded"`
	AllIn    bool        `json:"allIn"`
	Away     bool        `json:"away"`
	Leaving  bool        `json:"leaving"`
	Last     string      `json:"last"`
	Won      int64       `json:"won"`
	Hand     string      `json:"hand"`
	Best     []string    `json:"best"`
}

type holdemTableJSON struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Private    bool             `json:"private"`
	Owner      *authorJSON      `json:"owner"`
	BigBlind   int64            `json:"bigBlind"`
	SmallBlind int64            `json:"smallBlind"`
	MinBuyIn   int64            `json:"minBuyIn"`
	MaxBuyIn   int64            `json:"maxBuyIn"`
	Street     string           `json:"street"`
	Hand       int64            `json:"hand"`
	Button     int              `json:"button"`
	Turn       int              `json:"turn"`
	TurnMs     int64            `json:"turnMs"`
	Board      []string         `json:"board"`
	Pot        int64            `json:"pot"`
	CurrentBet int64            `json:"currentBet"`
	Version    int64            `json:"version"`
	Seats      []holdemSeatJSON `json:"seats"`
	Players    int              `json:"players"`
	You        int              `json:"you"`
	ToCall     int64            `json:"toCall"`
	RaiseLeast int64            `json:"raiseLeast"`
	RaiseMost  int64            `json:"raiseMost"`
	Invited    bool             `json:"invited"`
}

// holdemTables are tables with the people sitting at them.
func (a *api) holdemTables(r *http.Request, tables []model.HoldemTable) ([]holdemTableJSON, error) {
	var ids []string
	for _, t := range tables {
		ids = append(ids, t.OwnerID)
		for _, s := range t.Seats {
			if s.UserID != "" {
				ids = append(ids, s.UserID)
			}
		}
	}

	people, err := a.people(r, ids)
	if err != nil {
		return nil, err
	}

	person := func(id string) *authorJSON {
		p, ok := people[id]
		if !ok {
			return nil
		}
		out := toAuthorJSON(p)
		return &out
	}

	out := make([]holdemTableJSON, 0, len(tables))
	for _, t := range tables {
		table := holdemTableJSON{
			ID: t.ID, Name: t.Name, Private: t.Private, Owner: person(t.OwnerID), BigBlind: t.BigBlind,
			SmallBlind: t.SmallBlind, MinBuyIn: t.MinBuyIn, MaxBuyIn: t.MaxBuyIn, Street: t.Street, Hand: t.Hand,
			Button: t.Button, Turn: t.Turn, TurnMs: t.TurnMs, Board: nonNil(t.Board), Pot: t.Pot,
			CurrentBet: t.CurrentBet, Version: t.Version, Players: t.Players, You: t.You, ToCall: t.ToCall,
			RaiseLeast: t.RaiseLeast, RaiseMost: t.RaiseMost, Invited: t.Invited,
			Seats: make([]holdemSeatJSON, 0, len(t.Seats)),
		}
		for _, s := range t.Seats {
			table.Seats = append(table.Seats, holdemSeatJSON{
				Player: person(s.UserID), Stack: s.Stack, Bet: s.Bet, Cards: nonNil(s.Cards), HasCards: s.HasCards,
				Folded: s.Folded, AllIn: s.AllIn, Away: s.Away, Leaving: s.Leaving, Last: s.Last, Won: s.Won,
				Hand: s.Hand, Best: nonNil(s.Best),
			})
		}
		out = append(out, table)
	}

	return out, nil
}

func (a *api) writeHoldemTable(w http.ResponseWriter, r *http.Request, t model.HoldemTable) {
	tables, err := a.holdemTables(r, []model.HoldemTable{t})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, tables[0])
}

// writeHoldemSeat is a table after the caller's buy-in or payout and
// their balance; the table is null when it closed as they left.
func (a *api) writeHoldemSeat(w http.ResponseWriter, r *http.Request, t *model.HoldemTable, balance int64) {
	out := struct {
		Table   *holdemTableJSON `json:"table"`
		Balance int64            `json:"balance"`
	}{Balance: balance}

	if t != nil {
		tables, err := a.holdemTables(r, []model.HoldemTable{*t})
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		out.Table = &tables[0]
	}

	writeJSON(w, http.StatusOK, out)
}

// listHoldemTables are the tables of a section: "public", "mine",
// "invited".
func (a *api) listHoldemTables(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	tables, err := a.holdems.List(r.Context(), token, r.URL.Query().Get("section"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out, err := a.holdemTables(r, tables)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Tables []holdemTableJSON `json:"tables"`
	}{out})
}

func (a *api) createHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Private  bool   `json:"private"`
		Seats    int    `json:"seats"`
		BigBlind int64  `json:"bigBlind"`
		BuyIn    int64  `json:"buyIn"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	table, balance, err := a.holdems.Create(r.Context(), token, model.NewHoldemTable(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemSeat(w, r, table, balance)
}

func (a *api) quickHoldem(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		BigBlind int64 `json:"bigBlind"`
		BuyIn    int64 `json:"buyIn"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	table, balance, err := a.holdems.Quick(r.Context(), token, req.BigBlind, req.BuyIn)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemSeat(w, r, table, balance)
}

func (a *api) holdemTable(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	table, err := a.holdems.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// A player polls the table while it plays: the same version again is
	// told without the people.
	if since := r.URL.Query().Get("version"); since != "" && since == strconv.FormatInt(table.Version, 10) && table.TurnMs > 0 {
		writeJSON(w, http.StatusOK, struct {
			Version int64 `json:"version"`
			TurnMs  int64 `json:"turnMs"`
			Same    bool  `json:"same"`
		}{table.Version, table.TurnMs, true})
		return
	}

	a.writeHoldemTable(w, r, table)
}

func (a *api) sitHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	req := struct {
		Seat  *int  `json:"seat"`
		BuyIn int64 `json:"buyIn"`
	}{}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	seat := -1
	if req.Seat != nil {
		seat = *req.Seat
	}

	table, balance, err := a.holdems.Sit(r.Context(), token, r.PathValue("id"), seat, req.BuyIn)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemSeat(w, r, table, balance)
}

func (a *api) leaveHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	table, balance, err := a.holdems.Leave(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemSeat(w, r, table, balance)
}

var holdemActions = map[string]bool{"fold": true, "check": true, "call": true, "raise": true, "allin": true}

func (a *api) actHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Action string `json:"action"`
		Amount int64  `json:"amount"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}
	if !holdemActions[req.Action] {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "unknown action of poker"))
		return
	}

	table, err := a.holdems.Act(r.Context(), token, r.PathValue("id"), req.Action, req.Amount)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemTable(w, r, table)
}

func (a *api) backHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	table, err := a.holdems.Back(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemTable(w, r, table)
}

func (a *api) inviteHoldemTable(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Nicknames []string `json:"nicknames"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}
	if len(req.Nicknames) == 0 || len(req.Nicknames) > 50 {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "a table invites up to 50 people"))
		return
	}

	table, err := a.holdems.Invite(r.Context(), token, r.PathValue("id"), req.Nicknames)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeHoldemTable(w, r, table)
}
