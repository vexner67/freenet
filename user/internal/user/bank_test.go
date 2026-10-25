package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

var bankStart = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func day(n int) time.Time { return bankStart.Add(time.Duration(n) * 24 * time.Hour) }

func TestNewBank(t *testing.T) {
	t.Parallel()

	if _, err := user.NewBank(user.DefaultBank); err != nil {
		t.Fatalf("NewBank of the default bank: %v", err)
	}

	tests := []struct {
		name   string
		change func(b *user.Bank)
		want   error
	}{
		{"negative deposit rate", func(b *user.Bank) { b.DepositRate = -1 }, user.ErrBankRate},
		{"loan rate above 100 %", func(b *user.Bank) { b.LoanRate = user.BankOdds + 1 }, user.ErrBankRate},
		{"negative penalty", func(b *user.Bank) { b.PenaltyRate = -1 }, user.ErrBankRate},
		{"no deposits", func(b *user.Bank) { b.DepositMost = 0 }, user.ErrBankMost},
		{"no loans", func(b *user.Bank) { b.LoanMost = 0 }, user.ErrBankMost},
		{"no term", func(b *user.Bank) { b.LoanDays = 0 }, user.ErrBankDays},
		{"term above a year", func(b *user.Bank) { b.LoanDays = user.MaxBankDays + 1 }, user.ErrBankDays},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := user.DefaultBank
			tt.change(&b)
			if _, err := user.NewBank(b); !errors.Is(err, tt.want) {
				t.Errorf("NewBank = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBankAccount_Deposit(t *testing.T) {
	t.Parallel()

	b := user.Bank{DepositRate: 100, DepositMost: 1000, LoanRate: 100, LoanMost: 500, LoanDays: 14, PenaltyRate: 200}

	var a user.BankAccount
	a, err := a.Put(b, 100, bankStart)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// A day short of a day counts nothing; whole days compound 1 %.
	if got := a.At(b, day(1).Add(-time.Minute)).Deposit(); got != 10_000 {
		t.Errorf("deposit before a day = %d, want 10000", got)
	}
	if got := a.At(b, day(2)).Deposit(); got != 10_201 {
		t.Errorf("deposit after two days = %d, want 10201", got)
	}

	if _, err := a.Put(b, 901, bankStart); !errors.Is(err, user.ErrDepositMost) {
		t.Errorf("Put above the most = %v, want ErrDepositMost", err)
	}
	if _, err := a.Put(b, 0, bankStart); !errors.Is(err, user.ErrBankAmount) {
		t.Errorf("Put of nothing = %v, want ErrBankAmount", err)
	}
	if _, _, err := a.Take(b, 103, day(2)); !errors.Is(err, user.ErrDepositShort) {
		t.Errorf("Take of more = %v, want ErrDepositShort", err)
	}

	a, took, err := a.Take(b, 0, day(2))
	if err != nil || took != 102 {
		t.Fatalf("Take all = %d, %v; want 102", took, err)
	}
	if a.Deposit() != 1 {
		t.Errorf("deposit after taking all = %d, want the hundredth left", a.Deposit())
	}
}

func TestBankAccount_DepositStopsAtTheMost(t *testing.T) {
	t.Parallel()

	b := user.Bank{DepositRate: 1000, DepositMost: 110, LoanRate: 0, LoanMost: 1, LoanDays: 1, PenaltyRate: 0}

	a, err := user.BankAccount{}.Put(b, 100, bankStart)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if got := a.At(b, day(30)).DepositVotes(); got != 110 {
		t.Errorf("deposit after a month = %d, want the most, 110", got)
	}
}

func TestBankAccount_Loan(t *testing.T) {
	t.Parallel()

	b := user.Bank{DepositRate: 0, DepositMost: 1, LoanRate: 100, LoanMost: 500, LoanDays: 2, PenaltyRate: 1000}

	if _, err := (user.BankAccount{}).Borrow(b, 501, bankStart); !errors.Is(err, user.ErrLoanMost) {
		t.Errorf("Borrow above the most = %v, want ErrLoanMost", err)
	}
	if _, _, err := (user.BankAccount{}).Repay(b, 0, bankStart); !errors.Is(err, user.ErrNoLoan) {
		t.Errorf("Repay without a loan = %v, want ErrNoLoan", err)
	}

	a, err := user.BankAccount{}.Borrow(b, 100, bankStart)
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := a.Borrow(b, 1, bankStart); !errors.Is(err, user.ErrLoanActive) {
		t.Errorf("a second loan = %v, want ErrLoanActive", err)
	}

	// Two days at 1 %, then the third past the term at 10 %.
	if got := a.At(b, day(2)).Owed(); got != 103 {
		t.Errorf("owed by the term = %d, want 103 (102.01 rounded up)", got)
	}
	if got := a.At(b, day(3)).Owed(); got != 113 {
		t.Errorf("owed a day late = %d, want 113", got)
	}
	if a.OverdueAt(day(2)) || !a.OverdueAt(day(2).Add(time.Second)) {
		t.Error("the loan is overdue only past its term")
	}

	part, paid, err := a.Repay(b, 50, bankStart)
	if err != nil || paid != 50 || part.Owed() != 50 {
		t.Fatalf("Repay 50 = %d, owed %d, %v", paid, part.Owed(), err)
	}

	done, paid, err := part.Repay(b, 1000, bankStart)
	if err != nil || paid != 50 || done.HasLoan() {
		t.Fatalf("Repay more than owed = %d, loan %v, %v; want 50 and no loan", paid, done.HasLoan(), err)
	}
}

func TestBankAccount_DebtCap(t *testing.T) {
	t.Parallel()

	b := user.Bank{DepositRate: 0, DepositMost: 1, LoanRate: 10_000, LoanMost: 500, LoanDays: 1, PenaltyRate: 10_000}

	a, err := user.BankAccount{}.Borrow(b, 100, bankStart)
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	if got := a.At(b, day(365)).Owed(); got != 100*user.DebtCap {
		t.Errorf("owed after a year = %d, want %d", got, 100*user.DebtCap)
	}
}

func TestBankAccount_Collect(t *testing.T) {
	t.Parallel()

	b := user.Bank{DepositRate: 0, DepositMost: 1000, LoanRate: 0, LoanMost: 500, LoanDays: 1, PenaltyRate: 0}

	a, _ := user.BankAccount{}.Put(b, 30, bankStart)
	a, _ = a.Borrow(b, 100, bankStart)

	if _, got, _ := a.Collect(b, 1000, bankStart); got != 0 {
		t.Errorf("Collect before the term took %d", got)
	}

	after, fromBalance, fromDeposit := a.Collect(b, 50, day(2))
	if fromBalance != 50 || fromDeposit != 30 {
		t.Errorf("Collect = %d of the balance and %d of the deposit, want 50 and 30", fromBalance, fromDeposit)
	}
	if after.Owed() != 20 || after.Deposit() != 0 {
		t.Errorf("after Collect owed %d with deposit %d, want 20 and 0", after.Owed(), after.Deposit())
	}

	paid, fromBalance, fromDeposit := a.Collect(b, 500, day(2))
	if fromBalance != 100 || fromDeposit != 0 || paid.HasLoan() {
		t.Errorf("Collect of a rich page = %d and %d, loan %v", fromBalance, fromDeposit, paid.HasLoan())
	}
}
