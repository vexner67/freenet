package rest_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeBank struct {
	token  string
	action string
	amount int64
	set    model.BankSettings
	card   model.BankCard
	err    error
}

func (f *fakeBank) bank() model.Bank {
	return model.Bank{
		Settings: model.BankSettings{DepositRate: 20, DepositMost: 10_000, LoanRate: 100, LoanMost: 500, LoanDays: 14, PenaltyRate: 200},
		Account:  model.BankAccount{Deposit: 10_201, Debt: 10_300, Borrowed: 100, DueAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)},
	}
}

func (f *fakeBank) Get(_ context.Context, token string) (model.Bank, error) {
	f.token = token
	return f.bank(), f.err
}

func (f *fakeBank) Move(_ context.Context, token, action string, amount int64) (model.BankMove, error) {
	f.token, f.action, f.amount = token, action, amount
	return model.BankMove{Votes: amount, Balance: 40, Bank: f.bank()}, f.err
}

func (f *fakeBank) Card(_ context.Context, token string) (model.BankCard, error) {
	f.token = token
	return f.card, f.err
}

func (f *fakeBank) IssueCard(_ context.Context, token string) (model.BankCard, error) {
	f.token = token
	f.card = model.BankCard{Number: "1919000000000000", Cover: "classic", IssuedAt: time.Now()}
	return f.card, f.err
}

func (f *fakeBank) SetCardCover(_ context.Context, token, cover string) (model.BankCard, error) {
	f.token = token
	f.card.Cover = cover
	return f.card, f.err
}

func (f *fakeBank) Settings(_ context.Context, token string) (model.BankSettings, model.BankStats, error) {
	f.token = token
	return f.bank().Settings, model.BankStats{Deposits: 10_201, Depositors: 1, Debts: 10_300, Debtors: 1}, f.err
}

func (f *fakeBank) Set(_ context.Context, token string, settings model.BankSettings) error {
	f.token, f.set = token, settings
	return f.err
}

func TestBank(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/me/bank", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/me/bank", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	account := got["account"].(map[string]any)
	if w.Code != http.StatusOK || h.bank.token != "tok" || account["deposit"] != 10_201.0 ||
		account["dueAt"] != "2026-11-08T12:00:00Z" || got["settings"].(map[string]any)["loanDays"] != 14.0 {
		t.Fatalf("bank: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/bank/loan", `{"amount":100}`, "Authorization", "Bearer tok")
	moved := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.bank.action != "loan" || h.bank.amount != 100 || moved["balance"] != 40.0 || moved["bank"] == nil {
		t.Fatalf("loan: %d %v", w.Code, moved)
	}

	if w = h.do("POST", "/api/v1/me/bank/rob", `{"amount":1}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown action: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/bank/settings", "", "Authorization", "Bearer tok")
	if s := decode[map[string]any](t, w); w.Code != http.StatusOK || s["debtors"] != 1.0 {
		t.Fatalf("settings: %d %v", w.Code, s)
	}

	w = h.do("PUT", "/api/v1/bank/settings", `{"depositRate":30,"depositMost":5,"loanRate":1,"loanMost":2,"loanDays":3,"penaltyRate":4}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.bank.set.DepositRate != 30 || h.bank.set.LoanDays != 3 {
		t.Fatalf("set: %d %+v", w.Code, h.bank.set)
	}

	h.bank.err = failure.New(failure.Conflict, "pay back the loan you have first")
	if w = h.do("POST", "/api/v1/me/bank/loan", `{"amount":1}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("refused: %d", w.Code)
	}
}

func TestBankCard(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/me/bank/card", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/me/bank/card", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"card":null`) {
		t.Fatalf("no card: %d %s", w.Code, w.Body)
	}

	w = h.do("POST", "/api/v1/me/bank/card", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"number":"1919000000000000"`) {
		t.Fatalf("issue: %d %s", w.Code, w.Body)
	}

	w = h.do("PUT", "/api/v1/me/bank/card", `{"cover":"gold"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cover":"gold"`) {
		t.Fatalf("cover: %d %s", w.Code, w.Body)
	}
}
