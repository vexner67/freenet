package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestBankService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	now := time.Now().UTC()

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	banks := service.NewBankService(w.users, inmem.NewBankRepository(), w.users, inmem.TxManager{}, admins)

	if _, err = w.users.Credit(ctx, ann, 300, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}

	// A deposit takes votes and gives them back with the interest.
	if _, err = banks.PutDeposit(ctx, ann, 301, now); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("PutDeposit of more than ann has: %v", err)
	}
	put, err := banks.PutDeposit(ctx, ann, 200, now)
	if err != nil || put.Balance != 100 || put.View.Account.DepositVotes() != 200 {
		t.Fatalf("PutDeposit = %+v, %v", put, err)
	}

	later := now.Add(10 * 24 * time.Hour)
	view, err := banks.Bank(ctx, ann, later)
	if err != nil || view.Account.DepositVotes() != 204 {
		t.Fatalf("Bank ten days later = %d, %v; want 204", view.Account.DepositVotes(), err)
	}

	took, err := banks.TakeDeposit(ctx, ann, 0, later)
	if err != nil || took.Votes != 204 || took.Balance != 304 {
		t.Fatalf("TakeDeposit = %+v, %v", took, err)
	}

	// Loans are for pages a week old with a filled profile.
	if _, err = banks.Borrow(ctx, ann, 100, now); !errors.Is(err, user.ErrBankNotYet) {
		t.Fatalf("Borrow of a new page: %v", err)
	}
	if _, err = w.userSvc.UpdateProfile(ctx, ann, profile(t, "ann"), ""); err != nil {
		t.Fatal(err)
	}

	week := now.Add(user.BonusAge + time.Hour)
	lent, err := banks.Borrow(ctx, ann, 100, week)
	if err != nil || lent.Balance != 404 || lent.View.Account.Owed() != 100 {
		t.Fatalf("Borrow = %+v, %v", lent, err)
	}

	paid, err := banks.Repay(ctx, ann, 40, week)
	if err != nil || paid.Votes != 40 || paid.Balance != 364 || paid.View.Account.Owed() != 60 {
		t.Fatalf("Repay = %+v, %v", paid, err)
	}

	// Past its term the site takes the loan back itself.
	if n, err := banks.CollectDue(ctx, week); err != nil || n != 0 {
		t.Fatalf("CollectDue before the term = %d, %v", n, err)
	}

	due := week.Add(time.Duration(user.DefaultBank.LoanDays+1) * 24 * time.Hour)
	if n, err := banks.CollectDue(ctx, due); err != nil || n != 1 {
		t.Fatalf("CollectDue past the term = %d, %v", n, err)
	}
	if view, err = banks.Bank(ctx, ann, due); err != nil || view.Account.HasLoan() {
		t.Fatalf("the loan after CollectDue = %+v, %v", view.Account, err)
	}
	if got := w.votes(t, ann); got >= 364-60 {
		t.Errorf("ann has %d after the loan was collected, want less than %d", got, 364-60)
	}

	// Only the administrators read and tune the bank.
	tuned := user.DefaultBank
	tuned.LoanMost = 50
	if err = banks.SetBank(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetBank by ann: %v", err)
	}
	if _, err = banks.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	if err = banks.SetBank(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}
	if _, err = banks.Borrow(ctx, ann, 51, due); !errors.Is(err, user.ErrLoanMost) {
		t.Fatalf("Borrow above the new most: %v", err)
	}

	settings, err := banks.Settings(ctx, admin)
	if err != nil || settings.Bank.LoanMost != 50 || settings.Stats != (user.BankStats{}) {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
}
