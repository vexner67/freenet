package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PrimeRepository keeps Freenet Prime in memory: the subscriptions with
// the users, as in the table, and the prices apart.
type PrimeRepository struct {
	users *UserRepository

	mu     sync.RWMutex
	prices user.PrimePrices
}

func NewPrimeRepository(users *UserRepository) *PrimeRepository {
	return &PrimeRepository{
		users:  users,
		prices: user.DefaultPrimePrices,
	}
}

func (r *PrimeRepository) Prices(ctx context.Context) (user.PrimePrices, error) {
	if err := ctx.Err(); err != nil {
		return user.PrimePrices{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.prices, nil
}

func (r *PrimeRepository) SetPrices(ctx context.Context, prices user.PrimePrices) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.prices = prices

	return nil
}

func (r *PrimeRepository) Extend(ctx context.Context, userID uuid.UUID, period user.PrimePeriod, now time.Time) (time.Time, error) {
	var until time.Time

	err := r.change(ctx, userID, func(p user.Prime) user.Prime {
		until = p.Extended(period, now)
		p.Until = until

		return p
	})

	return until, err
}

func (r *PrimeRepository) SetInvisible(ctx context.Context, userID uuid.UUID, invisible bool) error {
	return r.change(ctx, userID, func(p user.Prime) user.Prime {
		p.Invisible = invisible
		return p
	})
}

func (r *PrimeRepository) change(ctx context.Context, userID uuid.UUID, change func(user.Prime) user.Prime) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.users.mu.Lock()
	defer r.users.mu.Unlock()

	u, ok := r.users.users[userID]
	if !ok {
		return user.ErrNotFound
	}

	r.users.users[userID] = *u.WithPrime(change(u.Prime()))

	return nil
}
