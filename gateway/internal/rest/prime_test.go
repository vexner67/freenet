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

type fakePrime struct {
	token, userID, period string
	prices                model.PrimePrices
	invisible             bool
	err                   error
}

func (f *fakePrime) Prices(context.Context) (model.PrimePrices, error) {
	return model.PrimePrices{Month: 150, Quarter: 400, Year: 1500}, nil
}

func (f *fakePrime) SetPrices(_ context.Context, token string, prices model.PrimePrices) error {
	f.token, f.prices = token, prices
	return f.err
}

func (f *fakePrime) Buy(_ context.Context, token, userID, period string) (model.PrimePurchase, error) {
	f.token, f.userID, f.period = token, userID, period
	return model.PrimePurchase{Until: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC), Balance: 50}, f.err
}

func (f *fakePrime) SetInvisible(_ context.Context, token string, invisible bool) error {
	f.token, f.invisible = token, invisible
	return f.err
}

func TestPrime(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/prime", "")
	if got := decode[map[string]int64](t, w); w.Code != http.StatusOK || got["month"] != 150 || got["year"] != 1500 {
		t.Fatalf("prices: %d %v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/prime/prices", `{"month":1,"quarter":2,"year":3}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent ||
		h.prime.prices != (model.PrimePrices{Month: 1, Quarter: 2, Year: 3}) {
		t.Fatalf("set prices: %d %+v", w.Code, h.prime.prices)
	}

	w = h.do("POST", "/api/v1/me/prime", `{"period":"month"}`, "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.prime.userID != "" || h.prime.period != "month" || got["until"] != "2026-11-21T12:00:00Z" || got["balance"] != float64(50) {
		t.Fatalf("buy: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/users/bob/prime", `{"period":"year"}`, "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.prime.userID != "u2" || h.prime.period != "year" {
		t.Fatalf("give: %d, %q", w.Code, h.prime.userID)
	}
	if w = h.do("POST", "/api/v1/users/nobody/prime", `{"period":"year"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("give to nobody: %d", w.Code)
	}

	if w = h.do("PUT", "/api/v1/me/prime/invisible", `{"invisible":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !h.prime.invisible {
		t.Fatalf("invisible: %d", w.Code)
	}

	h.prime.err = failure.New(failure.Conflict, "not enough votes")
	if w = h.do("POST", "/api/v1/me/prime", `{"period":"month"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("buy without votes: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/me/prime", `{"period":"month"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("buy without a token: %d", w.Code)
	}
}
