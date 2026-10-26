package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// FirmRepository keeps the economy of the enterprises in memory. The city
// is kept as its state, so that a change not saved is lost, as a
// transaction rolled back.
type FirmRepository struct {
	mu    sync.RWMutex
	rules user.FirmRules
	city  *user.CityState
}

func NewFirmRepository() *FirmRepository {
	return &FirmRepository{rules: user.DefaultFirmRules()}
}

func (r *FirmRepository) Rules(ctx context.Context) (user.FirmRules, error) {
	if err := ctx.Err(); err != nil {
		return user.FirmRules{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.rules, nil
}

func (r *FirmRepository) SetRules(ctx context.Context, rules user.FirmRules) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = rules

	return nil
}

func (r *FirmRepository) City(ctx context.Context, _ bool) (*user.City, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.city == nil {
		return user.NewCity(time.Now()), nil
	}

	return user.RestoreCity(*r.city), nil
}

func (r *FirmRepository) SaveCity(ctx context.Context, c *user.City) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	state := c.State()
	r.city = &state

	return nil
}
