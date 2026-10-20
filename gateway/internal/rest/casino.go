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
}

type casinoJSON struct {
	Price int64 `json:"price"`
	// Prizes are the votes of each prize: players do not see the odds.
	Prizes []int64 `json:"prizes"`
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
}

// casino is public: a guest sees what a spin costs and may win.
func (a *api) casino(w http.ResponseWriter, r *http.Request) {
	c, err := a.casinos.Get(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := casinoJSON{Price: c.Price, Prizes: make([]int64, 0, len(c.Prizes))}
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
