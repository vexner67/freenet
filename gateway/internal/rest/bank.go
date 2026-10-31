package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Bank is the bank of votes, which the user service keeps with them:
// people put deposits and take loans, the administrators tune it.
type Bank interface {
	Get(ctx context.Context, accessToken string) (model.Bank, error)
	// Move does action — "deposit", "withdraw", "loan" or "repay".
	Move(ctx context.Context, accessToken, action string, amount int64) (model.BankMove, error)
	Settings(ctx context.Context, accessToken string) (model.BankSettings, model.BankStats, error)
	Set(ctx context.Context, accessToken string, settings model.BankSettings) error
	Card(ctx context.Context, accessToken string) (model.BankCard, error)
	IssueCard(ctx context.Context, accessToken string) (model.BankCard, error)
	SetCardCover(ctx context.Context, accessToken, cover string) (model.BankCard, error)
}

// bankActions are what POST /api/v1/me/bank/{action} does.
var bankActions = map[string]bool{"deposit": true, "withdraw": true, "loan": true, "repay": true}

type bankSettingsJSON struct {
	DepositRate int   `json:"depositRate"`
	DepositMost int64 `json:"depositMost"`
	LoanRate    int   `json:"loanRate"`
	LoanMost    int64 `json:"loanMost"`
	LoanDays    int   `json:"loanDays"`
	PenaltyRate int   `json:"penaltyRate"`
}

type bankAccountJSON struct {
	Deposit  int64      `json:"deposit"`
	Debt     int64      `json:"debt"`
	Borrowed int64      `json:"borrowed"`
	DueAt    *time.Time `json:"dueAt"`
	Overdue  bool       `json:"overdue"`
}

type bankJSON struct {
	Settings bankSettingsJSON `json:"settings"`
	Account  bankAccountJSON  `json:"account"`
}

func toBankJSON(b model.Bank) bankJSON {
	a := b.Account
	account := bankAccountJSON{Deposit: a.Deposit, Debt: a.Debt, Borrowed: a.Borrowed, Overdue: a.Overdue}
	if !a.DueAt.IsZero() {
		due := a.DueAt.UTC()
		account.DueAt = &due
	}

	return bankJSON{Settings: bankSettingsJSON(b.Settings), Account: account}
}

// bank is "Банк" of "Мои Голоса": the deposit and the loan of the caller.
func (a *api) bank(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	b, err := a.banks.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toBankJSON(b))
}

func (a *api) moveBank(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	action := r.PathValue("action")
	if !bankActions[action] {
		a.writeError(w, r, failure.New(failure.NotFound, "unknown action of the bank"))
		return
	}

	var req struct {
		Amount int64 `json:"amount"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	moved, err := a.banks.Move(r.Context(), token, action, req.Amount)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Votes   int64    `json:"votes"`
		Balance int64    `json:"balance"`
		Bank    bankJSON `json:"bank"`
	}{moved.Votes, moved.Balance, toBankJSON(moved.Bank)})
}

type bankCardJSON struct {
	Number   string     `json:"number"`
	Cover    string     `json:"cover"`
	IssuedAt *time.Time `json:"issuedAt"`
}

// toBankCardJSON is null for a page without a card.
func toBankCardJSON(c model.BankCard) *bankCardJSON {
	if c.Number == "" {
		return nil
	}

	issued := c.IssuedAt.UTC()

	return &bankCardJSON{Number: c.Number, Cover: c.Cover, IssuedAt: &issued}
}

func (a *api) writeBankCard(w http.ResponseWriter, r *http.Request, c model.BankCard, err error) {
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Card *bankCardJSON `json:"card"`
	}{toBankCardJSON(c)})
}

// bankCard is the virtual card of the caller, if it was issued.
func (a *api) bankCard(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.banks.Card(r.Context(), token)
	a.writeBankCard(w, r, c, err)
}

func (a *api) issueBankCard(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.banks.IssueCard(r.Context(), token)
	a.writeBankCard(w, r, c, err)
}

func (a *api) setBankCardCover(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Cover string `json:"cover"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.banks.SetCardCover(r.Context(), token, req.Cover)
	a.writeBankCard(w, r, c, err)
}

// bankSettings is "Банк" of the admin panel: the rates and limits and
// how the bank does.
func (a *api) bankSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, stats, err := a.banks.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Settings   bankSettingsJSON `json:"settings"`
		Deposits   int64            `json:"deposits"`
		Depositors int64            `json:"depositors"`
		Debts      int64            `json:"debts"`
		Debtors    int64            `json:"debtors"`
	}{bankSettingsJSON(s), stats.Deposits, stats.Depositors, stats.Debts, stats.Debtors})
}

func (a *api) setBankSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req bankSettingsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.banks.Set(r.Context(), token, model.BankSettings(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
