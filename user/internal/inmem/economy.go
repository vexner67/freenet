package inmem

import (
	"context"
	"sync"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// EconomyRepository keeps the economy of the site in memory.
type EconomyRepository struct {
	mu      sync.RWMutex
	economy user.Economy
}

func NewEconomyRepository() *EconomyRepository {
	return &EconomyRepository{
		economy: user.DefaultEconomy,
	}
}

func (r *EconomyRepository) Economy(ctx context.Context) (user.Economy, error) {
	if err := ctx.Err(); err != nil {
		return user.Economy{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.economy, nil
}

func (r *EconomyRepository) SetEconomy(ctx context.Context, e user.Economy) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.economy = e

	return nil
}
