package model

import "time"

// BankSettings are the rates of the bank, in hundredths of a percent a
// day, the most a deposit grows to and a loan lends, in votes, and the
// term of a loan in days.
type BankSettings struct {
	DepositRate int
	DepositMost int64
	LoanRate    int
	LoanMost    int64
	LoanDays    int
	PenaltyRate int
}

// BankAccount is the deposit and the loan of the caller: the deposit and
// the debt in hundredths of a vote, what was borrowed and when it is due
// (zero without a loan).
type BankAccount struct {
	Deposit  int64
	Debt     int64
	Borrowed int64
	DueAt    time.Time
	Overdue  bool
}

// Bank is the bank as the caller sees it.
type Bank struct {
	Settings BankSettings
	Account  BankAccount
}

// BankMove is what a deposit or a loan moved: the votes, the balance
// after them and the bank.
type BankMove struct {
	Votes   int64
	Balance int64
	Bank    Bank
}

// BankCard is the virtual card of the caller; an empty Number means there
// is none yet.
type BankCard struct {
	Number   string
	Cover    string
	IssuedAt time.Time
}

// BankStats are the votes on deposits and owed on loans, in hundredths,
// and how many pages have each.
type BankStats struct {
	Deposits   int64
	Depositors int64
	Debts      int64
	Debtors    int64
}
