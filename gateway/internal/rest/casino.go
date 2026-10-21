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
	SetRocket(ctx context.Context, accessToken string, rocket model.CasinoRocket) error
	SetJackpot(ctx context.Context, accessToken string, jackpot int64) error
	LaunchRocket(ctx context.Context, accessToken string, amount int64, auto int) (model.RocketRound, error)
	Rocket(ctx context.Context, accessToken, id string) (model.RocketRound, error)
	CashRocket(ctx context.Context, accessToken, id string) (model.RocketRound, error)
}

type casinoJSON struct {
	Price int64 `json:"price"`
	// Prizes are the votes of each prize: players do not see the odds.
	Prizes []int64          `json:"prizes"`
	Bets   casinoBetsJSON   `json:"bets"`
	Rocket casinoRocketJSON `json:"rocket"`
	// Jackpot is the win the site celebrates over the whole screen.
	Jackpot int64 `json:"jackpot"`
}

// casinoRocketJSON is "Ракетка": the limits of the bets, the top
// multiplier in hundredths and, for the administrators, the return in
// hundredths of a percent.
type casinoRocketJSON struct {
	Least  int64 `json:"least"`
	Most   int64 `json:"most"`
	Return int   `json:"return,omitempty"`
	Top    int   `json:"top"`
}

// rocketRoundJSON is a round of "Ракетка": the crash point only once it
// is over.
type rocketRoundJSON struct {
	ID        string `json:"id"`
	Amount    int64  `json:"amount"`
	Auto      int    `json:"auto"`
	State     string `json:"state"`
	Cashed    int    `json:"cashed"`
	Payout    int64  `json:"payout"`
	Crash     int    `json:"crash"`
	ElapsedMS int64  `json:"elapsedMs"`
	Balance   int64  `json:"balance"`
}

// casinoBetsJSON are the limits of the bets and the multipliers with
// their chances.
type casinoBetsJSON struct {
	Least       int64               `json:"least"`
	Most        int64               `json:"most"`
	Multipliers []aimMultiplierJSON `json:"multipliers"`
}

// aimMultiplierJSON is a multiplier in hundredths (200 — ×2) and its
// chance of 10000.
type aimMultiplierJSON struct {
	Multiplier int `json:"multiplier"`
	Chance     int `json:"chance"`
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
	Rocket casinoRocketJSON  `json:"rocket"`
	// Rockets is how "Ракетка" does, as Wagers.
	Rockets casinoWagersJSON `json:"rockets"`
	Jackpot int64            `json:"jackpot"`
}

func toCasinoBetsJSON(b model.CasinoBets) casinoBetsJSON {
	out := casinoBetsJSON{Least: b.Least, Most: b.Most, Multipliers: make([]aimMultiplierJSON, 0, len(b.Multipliers))}
	for _, m := range b.Multipliers {
		out.Multipliers = append(out.Multipliers, aimMultiplierJSON(m))
	}

	return out
}

// casino is public: a guest sees what a spin costs and may win.
func (a *api) casino(w http.ResponseWriter, r *http.Request) {
	c, err := a.casinos.Get(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := casinoJSON{
		Price:   c.Price,
		Prizes:  make([]int64, 0, len(c.Prizes)),
		Bets:    toCasinoBetsJSON(c.Bets),
		Rocket:  casinoRocketJSON{Least: c.Rocket.Least, Most: c.Rocket.Most, Top: c.Rocket.Top},
		Jackpot: c.Jackpot,
	}
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
		Price:   s.Casino.Price,
		Prizes:  make([]casinoPrizeJSON, 0, len(s.Casino.Prizes)),
		Spins:   s.Spins,
		Spent:   s.Spent,
		Won:     s.Won,
		Bets:    toCasinoBetsJSON(s.Casino.Bets),
		Wagers:  casinoWagersJSON{Count: s.Wagers, Spent: s.WagerSpent, Won: s.WagerWon},
		Rocket:  casinoRocketJSON(s.Casino.Rocket),
		Rockets: casinoWagersJSON{Count: s.Rockets, Spent: s.RocketSpent, Won: s.RocketWon},
		Jackpot: s.Casino.Jackpot,
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

// setCasinoBets is for the administrators: the limits of the bets and the
// multipliers with their chances.
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

	bets := model.CasinoBets{Least: req.Least, Most: req.Most}
	for _, m := range req.Multipliers {
		bets.Multipliers = append(bets.Multipliers, model.AimMultiplier(m))
	}

	if err = a.casinos.SetBets(r.Context(), token, bets); err != nil {
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

// setCasinoRocket is for the administrators: the limits, the return and the
// top of "Ракетка".
func (a *api) setCasinoRocket(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req casinoRocketJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.casinos.SetRocket(r.Context(), token, model.CasinoRocket(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// setCasinoJackpot is for the administrators: the win the site celebrates
// over the whole screen.
func (a *api) setCasinoJackpot(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Jackpot int64 `json:"jackpot"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.casinos.SetJackpot(r.Context(), token, req.Jackpot); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// launchRocket bets votes on a rocket: the player cashes it before it
// explodes, or it cashes by itself at auto.
func (a *api) launchRocket(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Amount int64 `json:"amount"`
		Auto   int   `json:"auto"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	round, err := a.casinos.LaunchRocket(r.Context(), token, req.Amount, req.Auto)
	a.writeRocket(w, r, round, err)
}

// rocket is a round of the caller as it is now: the page asks while the
// rocket flies to learn it exploded.
func (a *api) rocket(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	round, err := a.casinos.Rocket(r.Context(), token, r.PathValue("id"))
	a.writeRocket(w, r, round, err)
}

func (a *api) cashRocket(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	round, err := a.casinos.CashRocket(r.Context(), token, r.PathValue("id"))
	a.writeRocket(w, r, round, err)
}

func (a *api) writeRocket(w http.ResponseWriter, r *http.Request, round model.RocketRound, err error) {
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, rocketRoundJSON{
		ID:        round.ID,
		Amount:    round.Amount,
		Auto:      round.Auto,
		State:     string(round.State),
		Cashed:    round.Cashed,
		Payout:    round.Payout,
		Crash:     round.Crash,
		ElapsedMS: round.ElapsedMS,
		Balance:   round.Balance,
	})
}
