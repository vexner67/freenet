package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MaxOverdue is how many loans past their term one run collects.
const MaxOverdue = 100

// BankRepository keeps the bank of the site and the accounts of pages.
type BankRepository interface {
	// Bank is user.DefaultBank until one is set.
	Bank(ctx context.Context) (user.Bank, error)
	SetBank(ctx context.Context, b user.Bank) error
	// Account is the account of userID, empty if it never had one; in a
	// transaction it stays so until the end — the deposits and loans of
	// one page wait for each other. user.ErrNotFound.
	Account(ctx context.Context, userID uuid.UUID) (user.BankAccount, error)
	SaveAccount(ctx context.Context, userID uuid.UUID, a user.BankAccount) error
	// Overdue are up to limit pages whose loans are past their term at
	// now, the longest due first.
	Overdue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	Stats(ctx context.Context) (user.BankStats, error)
}

// BankView is the bank as a page sees it at a time: its settings and the
// account with the interest counted.
type BankView struct {
	Bank    user.Bank
	Account user.BankAccount
}

// Banked is what a deposit or loan moved: the votes, the balance after
// it and the bank.
type Banked struct {
	Votes   int64
	Balance int64
	View    BankView
}

// BankSettings are the bank and how it does: for the administrators.
type BankSettings struct {
	Bank  user.Bank
	Stats user.BankStats
}

// BankService is the bank of votes: deposits grow out of the treasury,
// loans are lent out of it and paid back into it, and a loan past its
// term the site takes back itself.
type BankService struct {
	userRepo UserRepository
	banks    BankRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
}

func NewBankService(users UserRepository, banks BankRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *BankService {
	return &BankService{userRepo: users, banks: banks, wallets: wallets, tx: tx, admins: admins}
}

// change runs move on the account of userID at now in a transaction and
// saves what it made of it.
func (s *BankService) change(ctx context.Context, userID uuid.UUID, now time.Time, move func(ctx context.Context, b user.Bank, a user.BankAccount, got *Banked) (user.BankAccount, error)) (Banked, error) {
	var got Banked

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		b, err := s.banks.Bank(ctx)
		if err != nil {
			return errs.Wrap("s.banks.Bank", err)
		}

		a, err := s.banks.Account(ctx, userID)
		if err != nil {
			return errs.Wrap("s.banks.Account", err)
		}

		if a, err = move(ctx, b, a, &got); err != nil {
			return errs.Wrap("move", err)
		}

		got.View = BankView{Bank: b, Account: a.At(b, now)}

		return errs.Wrap("s.banks.SaveAccount", s.banks.SaveAccount(ctx, userID, a))
	})
	if err != nil {
		return Banked{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return got, nil
}

// Bank is the bank of userID at now.
func (s *BankService) Bank(ctx context.Context, userID uuid.UUID, now time.Time) (BankView, error) {
	b, err := s.banks.Bank(ctx)
	if err != nil {
		return BankView{}, errs.Wrap("s.banks.Bank", err)
	}

	a, err := s.banks.Account(ctx, userID)
	if err != nil {
		return BankView{}, errs.Wrap("s.banks.Account", err)
	}

	return BankView{Bank: b, Account: a.At(b, now)}, nil
}

// PutDeposit puts votes of userID on the deposit at now: they go into the
// treasury. user.ErrBankAmount, user.ErrDepositMost,
// user.ErrNotEnoughVotes.
func (s *BankService) PutDeposit(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (Banked, error) {
	return s.change(ctx, userID, now, func(ctx context.Context, b user.Bank, a user.BankAccount, got *Banked) (user.BankAccount, error) {
		a, err := a.Put(b, votes, now)
		if err != nil {
			return a, errs.Wrap("a.Put", err)
		}

		got.Votes = votes
		got.Balance, err = s.wallets.Spend(ctx, userID, votes, user.Entry(user.VoteDeposit, uuid.Nil, ""))

		return a, errs.Wrap("s.wallets.Spend", err)
	})
}

// TakeDeposit takes votes off the deposit of userID at now, every whole
// vote with votes 0; they come out of the treasury. user.ErrBankAmount,
// user.ErrDepositShort.
func (s *BankService) TakeDeposit(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (Banked, error) {
	return s.change(ctx, userID, now, func(ctx context.Context, b user.Bank, a user.BankAccount, got *Banked) (user.BankAccount, error) {
		a, took, err := a.Take(b, votes, now)
		if err != nil {
			return a, errs.Wrap("a.Take", err)
		}

		wallet, err := s.wallets.Credit(ctx, userID, took, 0, user.Entry(user.VoteDeposit, uuid.Nil, ""))
		if err != nil {
			return a, errs.Wrap("s.wallets.Credit", err)
		}

		got.Votes, got.Balance = took, wallet.Votes

		return a, nil
	})
}

// Borrow lends votes to userID at now out of the treasury;
// user.ErrBankNotYet for a page that may not borrow yet,
// user.ErrLoanActive, user.ErrBankAmount, user.ErrLoanMost.
func (s *BankService) Borrow(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (Banked, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return Banked{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = u.CheckBorrowAt(now); err != nil {
		return Banked{}, errs.Wrap("u.CheckBorrowAt", err)
	}

	return s.change(ctx, userID, now, func(ctx context.Context, b user.Bank, a user.BankAccount, got *Banked) (user.BankAccount, error) {
		a, err := a.Borrow(b, votes, now)
		if err != nil {
			return a, errs.Wrap("a.Borrow", err)
		}

		wallet, err := s.wallets.Credit(ctx, userID, votes, 0, user.Entry(user.VoteLoan, uuid.Nil, ""))
		if err != nil {
			return a, errs.Wrap("s.wallets.Credit", err)
		}

		got.Votes, got.Balance = votes, wallet.Votes

		return a, nil
	})
}

// Repay pays votes of the loan of userID back at now, all of it with
// votes 0; they go into the treasury. user.ErrNoLoan,
// user.ErrBankAmount, user.ErrNotEnoughVotes.
func (s *BankService) Repay(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (Banked, error) {
	return s.change(ctx, userID, now, func(ctx context.Context, b user.Bank, a user.BankAccount, got *Banked) (user.BankAccount, error) {
		a, paid, err := a.Repay(b, votes, now)
		if err != nil {
			return a, errs.Wrap("a.Repay", err)
		}

		got.Votes = paid
		got.Balance, err = s.wallets.Spend(ctx, userID, paid, user.Entry(user.VoteLoan, uuid.Nil, ""))

		return a, errs.Wrap("s.wallets.Spend", err)
	})
}

// CollectDue takes back up to MaxOverdue loans past their term at now:
// of the balance and the deposit of each, as much as is owed and there
// is. It says how many it took anything for.
func (s *BankService) CollectDue(ctx context.Context, now time.Time) (int, error) {
	due, err := s.banks.Overdue(ctx, now, MaxOverdue)
	if err != nil {
		return 0, errs.Wrap("s.banks.Overdue", err)
	}

	var (
		collected int
		failed    []error
	)

	for _, userID := range due {
		took, err := s.collect(ctx, userID, now)
		if err != nil {
			failed = append(failed, errs.Wrap("s.collect", err))
			continue
		}
		if took {
			collected++
		}
	}

	return collected, errors.Join(failed...)
}

func (s *BankService) collect(ctx context.Context, userID uuid.UUID, now time.Time) (bool, error) {
	var took bool

	_, err := s.change(ctx, userID, now, func(ctx context.Context, b user.Bank, a user.BankAccount, _ *Banked) (user.BankAccount, error) {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return a, errs.Wrap("s.userRepo.GetByID", err)
		}

		a, fromBalance, fromDeposit := a.Collect(b, u.Wallet().Votes, now)
		if fromBalance > 0 {
			if _, err = s.wallets.Spend(ctx, userID, fromBalance, user.Entry(user.VoteLoan, uuid.Nil, "overdue")); err != nil {
				return a, errs.Wrap("s.wallets.Spend", err)
			}
		}

		took = fromBalance+fromDeposit > 0

		return a, nil
	})

	return took, errs.Wrap("s.change", err)
}

// Settings are the bank and its stats; user.ErrNotAdmin.
func (s *BankService) Settings(ctx context.Context, adminID uuid.UUID) (BankSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return BankSettings{}, errs.Wrap("checkAdmin", err)
	}

	b, err := s.banks.Bank(ctx)
	if err != nil {
		return BankSettings{}, errs.Wrap("s.banks.Bank", err)
	}

	stats, err := s.banks.Stats(ctx)
	if err != nil {
		return BankSettings{}, errs.Wrap("s.banks.Stats", err)
	}

	return BankSettings{Bank: b, Stats: stats}, nil
}

// SetBank changes the rates and limits of the bank; user.ErrNotAdmin.
func (s *BankService) SetBank(ctx context.Context, adminID uuid.UUID, b user.Bank) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.banks.SetBank", s.banks.SetBank(ctx, b))
}
