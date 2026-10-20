package inmem

import (
	"context"
	"sync"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// DecorRepository keeps the New Year decoration of the site in memory.
type DecorRepository struct {
	mu    sync.RWMutex
	decor user.Decor
}

func NewDecorRepository() *DecorRepository {
	return &DecorRepository{
		decor: user.DecorAuto,
	}
}

func (r *DecorRepository) Decor(ctx context.Context) (user.Decor, error) {
	if err := ctx.Err(); err != nil {
		return "", errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.decor, nil
}

func (r *DecorRepository) SetDecor(ctx context.Context, decor user.Decor) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.decor = decor

	return nil
}
