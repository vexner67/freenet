package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Casino is the casino of the site, which the user service keeps with the
// votes: people spin it, the administrators tune the odds.
type Casino interface {
	Get(ctx context.Context) (model.Casino, error)
	Settings(ctx context.Context, accessToken string) (model.CasinoSettings, error)
	Set(ctx context.Context, accessToken string, casino model.Casino) error
	Spin(ctx context.Context, accessToken string) (model.CasinoSpin, error)
	SetBets(ctx context.Context, accessToken string, bets model.CasinoBets) error
	Bet(ctx context.Context, accessToken string, amount int64, multiplier int) (model.CasinoWager, error)
}

type casinoJSON struct {
	Price int64 `json:"price"`
	// Prizes are the votes of each prize: players do not see the odds.
	Prizes []int64        `json:"prizes"`
	Bets   casinoBetsJSON `json:"bets"`
}

// casinoBetsJSON are the limits of the bets, the percent they give back
// and the multipliers, in hundredths.
type casinoBetsJSON struct {
	Least       int64 `json:"least"`
	Most        int64 `json:"most"`
	Return      int   `json:"return"`
	Multipliers []int `json:"multipliers,omitempty"`
}

// casinoWagersJSON is how the bets do.
type casinoWagersJSON struct {
	Count int64 `json:"count"`
	Spent int64 `json:"spent"`
	Won   int64 `json:"won"`
}

type casinoPrizeJSON struct {
	Votes  int64 `json:"votes"`
	Weight int   `json:"weight"`
}

type casinoSettingsJSON struct {
	Price  int64             `json:"price"`
	Prizes []casinoPrizeJSON `json:"prizes"`
	Spins  int64             `json:"spins"`
	Spent  int64             `json:"spent"`
	Won    int64             `json:"won"`
	Bets   casinoBetsJSON    `json:"bets"`
	Wagers casinoWagersJSON  `json:"wagers"`
}

func toCasinoBetsJSON(b model.CasinoBets) casinoBetsJSON {
	return casinoBetsJSON{Least: b.Least, Most: b.Most, Return: b.Return, Multipliers: b.Multipliers}
}

// casino is public: a guest sees what a spin costs and may win.
func (a *api) casino(w http.ResponseWriter, r *http.Request) {
	c, err := a.casinos.Get(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := casinoJSON{Price: c.Price, Prizes: make([]int64, 0, len(c.Prizes)), Bets: toCasinoBetsJSON(c.Bets)}
	for _, p := range c.Prizes {
		out.Prizes = append(out.Prizes, p.Votes)
	}

	writeJSON(w, http.StatusOK, out)
}

// casinoSettings is "Казино" of the admin panel: the odds and how the
// casino does.
func (a *api) casinoSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, err := a.casinos.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := casinoSettingsJSON{
		Price:  s.Casino.Price,
		Prizes: make([]casinoPrizeJSON, 0, len(s.Casino.Prizes)),
		Spins:  s.Spins,
		Spent:  s.Spent,
		Won:    s.Won,
		Bets:   toCasinoBetsJSON(s.Casino.Bets),
		Wagers: casinoWagersJSON{Count: s.Wagers, Spent: s.WagerSpent, Won: s.WagerWon},
	}
	for _, p := range s.Casino.Prizes {
		out.Prizes = append(out.Prizes, casinoPrizeJSON(p))
	}

	writeJSON(w, http.StatusOK, out)
}

func (a *api) setCasino(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Price  int64             `json:"price"`
		Prizes []casinoPrizeJSON `json:"prizes"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	casino := model.Casino{Price: req.Price}
	for _, p := range req.Prizes {
		casino.Prizes = append(casino.Prizes, model.CasinoPrize(p))
	}

	if err = a.casinos.Set(r.Context(), token, casino); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) spinCasino(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	spin, err := a.casinos.Spin(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Prize   int   `json:"prize"`
		Votes   int64 `json:"votes"`
		Balance int64 `json:"balance"`
	}{spin.Prize, spin.Votes, spin.Balance})
}

// setCasinoBets is for the administrators: the limits and the return of
// the bets.
func (a *api) setCasinoBets(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req casinoBetsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.casinos.SetBets(r.Context(), token, model.CasinoBets{Least: req.Least, Most: req.Most, Return: req.Return}); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// betCasino bets votes on a multiplier: they come back multiplied or not
// at all.
func (a *api) betCasino(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Amount     int64 `json:"amount"`
		Multiplier int   `json:"multiplier"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	wager, err := a.casinos.Bet(r.Context(), token, req.Amount, req.Multiplier)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Won     bool  `json:"won"`
		Roll    int   `json:"roll"`
		Chance  int   `json:"chance"`
		Votes   int64 `json:"votes"`
		Balance int64 `json:"balance"`
	}{wager.Won, wager.Roll, wager.Chance, wager.Votes, wager.Balance})
}
