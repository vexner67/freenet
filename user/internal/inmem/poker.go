package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PokerRepository keeps the table of "Покер" and its hands in memory.
type PokerRepository struct {
	mu    sync.Mutex
	poker user.Poker
	stats user.PokerStats
	hands map[uuid.UUID]user.PokerHand
}

func NewPokerRepository() *PokerRepository {
	return &PokerRepository{poker: user.DefaultPoker(), hands: make(map[uuid.UUID]user.PokerHand)}
}

func (r *PokerRepository) Poker(ctx context.Context) (user.Poker, error) {
	if err := ctx.Err(); err != nil {
		return user.Poker{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.poker, nil
}

func (r *PokerRepository) SetPoker(ctx context.Context, p user.Poker) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.poker = p

	return nil
}

func (r *PokerRepository) Stats(ctx context.Context) (user.PokerStats, error) {
	if err := ctx.Err(); err != nil {
		return user.PokerStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.stats, nil
}

func (r *PokerRepository) Count(ctx context.Context, hands, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Hands += hands
	r.stats.Spent += spent
	r.stats.Won += won

	return nil
}

func (r *PokerRepository) AddHand(ctx context.Context, h *user.PokerHand) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, other := range r.hands {
		if other.UserID() == h.UserID() && !other.Over() {
			return user.ErrPokerOpen
		}
	}
	r.hands[h.ID()] = *h

	return nil
}

func (r *PokerRepository) SetOver(ctx context.Context, h *user.PokerHand) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.hands[h.ID()]; !ok {
		return user.ErrPokerNotFound
	}
	r.hands[h.ID()] = *h

	return nil
}

func (r *PokerRepository) Hand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	h, ok := r.hands[id]
	if !ok {
		return nil, user.ErrPokerNotFound
	}

	return &h, nil
}

// LockHand is Hand: there are no transactions in memory.
func (r *PokerRepository) LockHand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error) {
	return r.Hand(ctx, id)
}

func (r *PokerRepository) Dealt(ctx context.Context, userID uuid.UUID) (*user.PokerHand, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, h := range r.hands {
		if h.UserID() == userID && !h.Over() {
			return &h, nil
		}
	}

	return nil, user.ErrPokerNotFound
}

func (r *PokerRepository) DeleteOver(ctx context.Context, userID uuid.UUID, before time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, h := range r.hands {
		if h.UserID() == userID && h.Over() && h.StartedAt().Before(before) {
			delete(r.hands, id)
		}
	}

	return nil
}
