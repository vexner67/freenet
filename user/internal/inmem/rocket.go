package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// RocketRepository keeps the rounds of "Ракетка" in memory.
type RocketRepository struct {
	mu     sync.Mutex
	rounds map[uuid.UUID]user.RocketRound
}

func NewRocketRepository() *RocketRepository {
	return &RocketRepository{rounds: make(map[uuid.UUID]user.RocketRound)}
}

func (r *RocketRepository) Save(ctx context.Context, round *user.RocketRound) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rounds[round.ID()] = *round

	return nil
}

func (r *RocketRepository) Get(ctx context.Context, id uuid.UUID) (*user.RocketRound, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	round, ok := r.rounds[id]
	if !ok {
		return nil, user.ErrRocketNotFound
	}

	return &round, nil
}

// Lock is Get: there are no transactions in memory.
func (r *RocketRepository) Lock(ctx context.Context, id uuid.UUID) (*user.RocketRound, error) {
	return r.Get(ctx, id)
}

func (r *RocketRepository) SetCashed(ctx context.Context, round *user.RocketRound) error {
	return r.Save(ctx, round)
}

func (r *RocketRepository) DeleteBefore(ctx context.Context, userID uuid.UUID, before time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, round := range r.rounds {
		if round.UserID() == userID && round.StartedAt().Before(before) {
			delete(r.rounds, id)
		}
	}

	return nil
}
