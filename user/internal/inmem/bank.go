package inmem

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// BankRepository keeps the bank of the site and the accounts of pages in
// memory.
type BankRepository struct {
	mu       sync.RWMutex
	bank     user.Bank
	accounts map[uuid.UUID]user.BankAccount
}

func NewBankRepository() *BankRepository {
	return &BankRepository{
		bank:     user.DefaultBank,
		accounts: map[uuid.UUID]user.BankAccount{},
	}
}

func (r *BankRepository) Bank(ctx context.Context) (user.Bank, error) {
	if err := ctx.Err(); err != nil {
		return user.Bank{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.bank, nil
}

func (r *BankRepository) SetBank(ctx context.Context, b user.Bank) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.bank = b

	return nil
}

func (r *BankRepository) Account(ctx context.Context, userID uuid.UUID) (user.BankAccount, error) {
	if err := ctx.Err(); err != nil {
		return user.BankAccount{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.accounts[userID], nil
}

func (r *BankRepository) SaveAccount(ctx context.Context, userID uuid.UUID, a user.BankAccount) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.accounts[userID] = a

	return nil
}

func (r *BankRepository) Overdue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var ids []uuid.UUID
	for id, a := range r.accounts {
		if a.OverdueAt(now) {
			ids = append(ids, id)
		}
	}

	slices.SortFunc(ids, func(x, y uuid.UUID) int { return r.accounts[x].DueAt().Compare(r.accounts[y].DueAt()) })

	return ids[:min(len(ids), limit)], nil
}

func (r *BankRepository) Stats(ctx context.Context) (user.BankStats, error) {
	if err := ctx.Err(); err != nil {
		return user.BankStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var stats user.BankStats
	for _, a := range r.accounts {
		if a.Deposit() > 0 {
			stats.Deposits += a.Deposit()
			stats.Depositors++
		}
		if a.HasLoan() {
			stats.Debts += a.Debt()
			stats.Debtors++
		}
	}

	return stats, nil
}
