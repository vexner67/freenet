package inmem

import (
	"context"
	"sync"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CasinoRepository keeps the casino of the site in memory.
type CasinoRepository struct {
	mu     sync.RWMutex
	casino user.Casino
	stats  user.CasinoStats
}

func NewCasinoRepository() *CasinoRepository {
	return &CasinoRepository{
		casino: user.DefaultCasino(),
	}
}

func (r *CasinoRepository) Casino(ctx context.Context) (user.Casino, error) {
	if err := ctx.Err(); err != nil {
		return user.Casino{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.casino, nil
}

func (r *CasinoRepository) SetCasino(ctx context.Context, casino user.Casino) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.casino = casino

	return nil
}

func (r *CasinoRepository) CountSpin(ctx context.Context, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Spins++
	r.stats.Spent += spent
	r.stats.Won += won

	return nil
}

func (r *CasinoRepository) Stats(ctx context.Context) (user.CasinoStats, error) {
	if err := ctx.Err(); err != nil {
		return user.CasinoStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.stats, nil
}
