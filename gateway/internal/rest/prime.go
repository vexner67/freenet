package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Prime is Freenet Prime, the subscription of the site the user service
// sells for votes: for oneself or as a present.
type Prime interface {
	Prices(ctx context.Context) (model.PrimePrices, error)
	SetPrices(ctx context.Context, accessToken string, prices model.PrimePrices) error
	// Buy buys period ("month", "quarter", "year") for userID, "" for the
	// caller.
	Buy(ctx context.Context, accessToken, userID, period string) (model.PrimePurchase, error)
	SetInvisible(ctx context.Context, accessToken string, invisible bool) error
}

type primePricesJSON struct {
	Month   int64 `json:"month"`
	Quarter int64 `json:"quarter"`
	Year    int64 `json:"year"`
}

type primePurchaseJSON struct {
	Until   string `json:"until"`
	Balance int64  `json:"balance"`
}

type buyPrimeRequest struct {
	Period string `json:"period"`
}

// primePrices is public: what Prime costs.
func (a *api) primePrices(w http.ResponseWriter, r *http.Request) {
	prices, err := a.primes.Prices(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, primePricesJSON(prices))
}

// setPrimePrices is "Freenet Prime" of the admin panel.
func (a *api) setPrimePrices(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req primePricesJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.primes.SetPrices(r.Context(), token, model.PrimePrices(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// buyPrime buys Prime for the caller.
func (a *api) buyPrime(w http.ResponseWriter, r *http.Request) {
	a.sellPrime(w, r, func(string) (string, error) { return "", nil })
}

// givePrime buys Prime as a present for the person of the path, who hears
// of it.
func (a *api) givePrime(w http.ResponseWriter, r *http.Request) {
	a.sellPrime(w, r, func(token string) (string, error) {
		recipient, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))

		return recipient.ID, err
	})
}

// sellPrime buys Prime for whom recipient names, "" for the caller.
func (a *api) sellPrime(w http.ResponseWriter, r *http.Request, recipient func(token string) (string, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req buyPrimeRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	userID, err := recipient(token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	bought, err := a.primes.Buy(r.Context(), token, userID, req.Period)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, primePurchaseJSON{Until: timeString(bought.Until), Balance: bought.Balance})
}

// setPrimeInvisible is "Невидимка": a member of Prime hides when they are
// online.
func (a *api) setPrimeInvisible(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Invisible bool `json:"invisible"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.primes.SetInvisible(r.Context(), token, req.Invisible); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
