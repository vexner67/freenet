package inmem

import (
	"context"
	"sync"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// VoteRateRepository keeps the rate of a vote in memory.
type VoteRateRepository struct {
	mu   sync.RWMutex
	rate user.VoteRate
}

func NewVoteRateRepository() *VoteRateRepository {
	return &VoteRateRepository{
		rate: user.DefaultVoteRate,
	}
}

func (r *VoteRateRepository) Rate(ctx context.Context) (user.VoteRate, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.rate, nil
}

func (r *VoteRateRepository) SetRate(ctx context.Context, rate user.VoteRate) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rate = rate

	return nil
}
