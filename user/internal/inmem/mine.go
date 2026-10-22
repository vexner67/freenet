package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MineRepository keeps the mine of the site and its miners in memory.
type MineRepository struct {
	mu     sync.RWMutex
	mine   user.Mine
	miners map[uuid.UUID]user.Miner
	stats  user.MineStats
}

func NewMineRepository() *MineRepository {
	return &MineRepository{
		mine:   user.DefaultMine(),
		miners: map[uuid.UUID]user.Miner{},
	}
}

func (r *MineRepository) Mine(ctx context.Context) (user.Mine, error) {
	if err := ctx.Err(); err != nil {
		return user.Mine{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.mine, nil
}

func (r *MineRepository) SetMine(ctx context.Context, mine user.Mine) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.mine = mine

	return nil
}

func (r *MineRepository) Miner(ctx context.Context, userID uuid.UUID, fresh user.Miner) (user.Miner, error) {
	if err := ctx.Err(); err != nil {
		return user.Miner{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.miners[userID]
	if !ok {
		r.miners[userID] = fresh

		return fresh, nil
	}

	return m, nil
}

func (r *MineRepository) SaveMiner(ctx context.Context, userID uuid.UUID, m user.Miner) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.miners[userID] = m

	return nil
}

func (r *MineRepository) CountDig(ctx context.Context, chest int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Digs++
	r.stats.Chests += chest

	return nil
}

func (r *MineRepository) CountSale(ctx context.Context, votes int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Sold += votes

	return nil
}

func (r *MineRepository) Stats(ctx context.Context) (user.MineStats, error) {
	if err := ctx.Err(); err != nil {
		return user.MineStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := r.stats
	for _, m := range r.miners {
		if m.Digs() > 0 {
			stats.Miners++
		}
	}

	return stats, nil
}
