package user

import (
	"errors"
	"time"
)

// The bank of votes: a deposit grows by a percent a day, and a loan is
// paid back with a percent a day by its term, and after it with a larger
// one, which the site takes from the balance itself.
const (
	// BankOdds is a whole percent a day: the rates are in hundredths of
	// a percent.
	BankOdds = 10_000
	// MaxBankDays is the longest term of a loan.
	MaxBankDays = 365
	// DebtCap is how many times what was borrowed a debt grows to at
	// most, however late it is.
	DebtCap = 3
	// bankDay is the step the interest is counted in: whole days only.
	bankDay = 24 * time.Hour
)

var (
	ErrBankRate     = errors.New("a rate must be from 0 to 100 percent a day")
	ErrBankMost     = errors.New("a limit of the bank must be at least 1 vote")
	ErrBankDays     = errors.New("the term of a loan is from 1 to 365 days")
	ErrBankAmount   = errors.New("the bank takes and gives at least 1 vote")
	ErrDepositMost  = errors.New("the deposit cannot be larger")
	ErrDepositShort = errors.New("there are not so many votes on the deposit")
	ErrLoanMost     = errors.New("the loan cannot be larger")
	ErrLoanActive   = errors.New("pay back the loan you have first")
	ErrNoLoan       = errors.New("you have no loan")
	ErrBankNotYet   = errors.New("loans are for pages a week old with a filled profile")
)

// Bank is what the administrators tune of the bank: the rate of deposits
// and the most that may be put on a deposit and earns, the rate of loans, the most a loan
// lends, its term, and the rate of a loan past its term. Rates are in
// hundredths of a percent a day.
type Bank struct {
	DepositRate int
	DepositMost int64
	LoanRate    int
	LoanMost    int64
	LoanDays    int
	PenaltyRate int
}

// DefaultBank is the bank until the administrators change it: 0.2 % a day
// for deposits up to 10 000, loans up to 500 at 1 % a day for two weeks,
// then 2 % a day.
var DefaultBank = Bank{DepositRate: 20, DepositMost: 10_000, LoanRate: 100, LoanMost: 500, LoanDays: 14, PenaltyRate: 200}

// NewBank checks a bank; ErrBankRate, ErrBankMost, ErrBankDays.
func NewBank(b Bank) (Bank, error) {
	for _, rate := range []int{b.DepositRate, b.LoanRate, b.PenaltyRate} {
		if rate < 0 || rate > BankOdds {
			return Bank{}, ErrBankRate
		}
	}
	for _, most := range []int64{b.DepositMost, b.LoanMost} {
		if most < 1 || most > MaxAmount {
			return Bank{}, ErrBankMost
		}
	}
	if b.LoanDays < 1 || b.LoanDays > MaxBankDays {
		return Bank{}, ErrBankDays
	}

	return b, nil
}

// BankAccount is the deposit and the loan of a page: the deposit and the
// debt in hundredths of a vote, each with the time its interest is
// counted up to; what was borrowed, in votes, and when it is due.
type BankAccount struct {
	deposit   int64
	depositAt time.Time
	debt      int64
	debtAt    time.Time
	borrowed  int64
	dueAt     time.Time
}

// RestoreBankAccount is an account as storage keeps it.
func RestoreBankAccount(deposit int64, depositAt time.Time, debt int64, debtAt time.Time, borrowed int64, dueAt time.Time) BankAccount {
	return BankAccount{deposit: deposit, depositAt: depositAt, debt: debt, debtAt: debtAt, borrowed: borrowed, dueAt: dueAt}
}

func (a BankAccount) Deposit() int64       { return a.deposit }
func (a BankAccount) DepositAt() time.Time { return a.depositAt }
func (a BankAccount) Debt() int64          { return a.debt }
func (a BankAccount) DebtAt() time.Time    { return a.debtAt }
func (a BankAccount) Borrowed() int64      { return a.borrowed }
func (a BankAccount) DueAt() time.Time     { return a.dueAt }

// DepositVotes are the whole votes of the deposit, what may be taken out.
func (a BankAccount) DepositVotes() int64 { return a.deposit / 100 }

// Owed is what the loan takes to pay back, in whole votes.
func (a BankAccount) Owed() int64 { return (a.debt + 99) / 100 }

// HasLoan tells whether a loan waits to be paid back.
func (a BankAccount) HasLoan() bool { return a.debt > 0 }

// OverdueAt tells whether the loan is past its term at now.
func (a BankAccount) OverdueAt(now time.Time) bool { return a.HasLoan() && now.After(a.dueAt) }

// At is the account with the interest of the whole days to now counted:
// a deposit earns on as much of it as the most a deposit may be, and what
// it earns above that keeps piling up; a debt grows until DebtCap times
// what was borrowed.
func (a BankAccount) At(b Bank, now time.Time) BankAccount {
	if a.deposit > 0 {
		days := wholeDays(a.depositAt, now)
		for range days {
			earning := min(a.deposit, b.DepositMost*100)
			a.deposit = min(a.deposit+earning*int64(b.DepositRate)/BankOdds, MaxAmount*100)
		}
		a.depositAt = a.depositAt.Add(time.Duration(days) * bankDay)
	}

	if a.debt > 0 {
		days := wholeDays(a.debtAt, now)
		most := max(a.borrowed*100*DebtCap, a.debt)
		for i := range days {
			if a.debt >= most {
				break
			}
			rate := int64(b.LoanRate)
			if day := a.debtAt.Add(time.Duration(i+1) * bankDay); day.After(a.dueAt) {
				rate = int64(b.PenaltyRate)
			}
			a.debt = min(a.debt+(a.debt*rate+BankOdds-1)/BankOdds, most)
		}
		a.debtAt = a.debtAt.Add(time.Duration(days) * bankDay)
	}

	return a
}

// wholeDays are the whole days from since to now, none before since and
// no more than ten years.
func wholeDays(since, now time.Time) int {
	if !now.After(since) {
		return 0
	}

	return int(min(now.Sub(since)/bankDay, 3650))
}

// Put puts votes on the deposit at now; ErrBankAmount, ErrDepositMost.
// A deposit begins its days anew when it was empty.
func (a BankAccount) Put(b Bank, votes int64, now time.Time) (BankAccount, error) {
	if votes < 1 || votes > MaxAmount {
		return a, ErrBankAmount
	}

	a = a.At(b, now)
	if a.deposit+votes*100 > b.DepositMost*100 {
		return a, ErrDepositMost
	}
	if a.deposit == 0 {
		a.depositAt = now
	}
	a.deposit += votes * 100

	return a, nil
}

// Take takes votes off the deposit at now, every whole vote with votes 0,
// and says how many; ErrBankAmount, ErrDepositShort.
func (a BankAccount) Take(b Bank, votes int64, now time.Time) (BankAccount, int64, error) {
	a = a.At(b, now)
	if votes == 0 {
		votes = a.DepositVotes()
	}
	if votes < 1 || votes > MaxAmount {
		return a, 0, ErrBankAmount
	}
	if votes > a.DepositVotes() {
		return a, 0, ErrDepositShort
	}
	a.deposit -= votes * 100

	return a, votes, nil
}

// Borrow lends votes at now, due in the term of the bank; ErrLoanActive
// while a loan waits, ErrBankAmount, ErrLoanMost.
func (a BankAccount) Borrow(b Bank, votes int64, now time.Time) (BankAccount, error) {
	a = a.At(b, now)
	if a.HasLoan() {
		return a, ErrLoanActive
	}
	if votes < 1 || votes > MaxAmount {
		return a, ErrBankAmount
	}
	if votes > b.LoanMost {
		return a, ErrLoanMost
	}

	a.debt, a.debtAt, a.borrowed, a.dueAt = votes*100, now, votes, now.Add(time.Duration(b.LoanDays)*bankDay)

	return a, nil
}

// Repay pays votes of the loan back at now, all of it with votes 0 or
// more than is owed, and says how many votes it took; ErrNoLoan,
// ErrBankAmount.
func (a BankAccount) Repay(b Bank, votes int64, now time.Time) (BankAccount, int64, error) {
	a = a.At(b, now)
	if !a.HasLoan() {
		return a, 0, ErrNoLoan
	}
	if votes < 0 || votes > MaxAmount {
		return a, 0, ErrBankAmount
	}
	if votes == 0 || votes >= a.Owed() {
		votes = a.Owed()
	}

	return a.paid(votes), votes, nil
}

// paid is the account with votes of its debt paid; the whole of it ends
// the loan.
func (a BankAccount) paid(votes int64) BankAccount {
	if votes >= a.Owed() {
		a.debt, a.borrowed, a.debtAt, a.dueAt = 0, 0, time.Time{}, time.Time{}

		return a
	}
	a.debt -= votes * 100

	return a
}

// Collect is what the site takes at now for a loan past its term: first
// of balance, then of the whole votes of the deposit, as much as is owed
// and there is; and the account after it.
func (a BankAccount) Collect(b Bank, balance int64, now time.Time) (acct BankAccount, fromBalance, fromDeposit int64) {
	a = a.At(b, now)
	if !a.OverdueAt(now) {
		return a, 0, 0
	}

	fromBalance = min(max(balance, 0), a.Owed())
	a = a.paid(fromBalance)

	if a.HasLoan() {
		fromDeposit = min(a.DepositVotes(), a.Owed())
		a.deposit -= fromDeposit * 100
		a = a.paid(fromDeposit)
	}

	return a, fromBalance, fromDeposit
}

// CheckBorrowAt is ErrBankNotYet unless u may borrow at now: the pages
// that may take the daily bonus.
func (u *User) CheckBorrowAt(now time.Time) error {
	if err := u.CheckBonusAt(now); err != nil {
		return ErrBankNotYet
	}

	return nil
}

// BankStats is how the bank does: votes on deposits and owed on loans,
// in hundredths, as counted last, and how many pages have each.
type BankStats struct {
	Deposits   int64
	Depositors int64
	Debts      int64
	Debtors    int64
}
