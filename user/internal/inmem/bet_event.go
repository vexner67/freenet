package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// BetEventRepository keeps the events and their stakes in memory.
type BetEventRepository struct {
	mu     sync.RWMutex
	events map[uuid.UUID]*user.BetEvent
	// stakes are the stakes of each event, in the order they were first
	// made.
	stakes map[uuid.UUID][]user.Stake
	now    func() time.Time
}

func NewBetEventRepository() *BetEventRepository {
	return &BetEventRepository{
		events: make(map[uuid.UUID]*user.BetEvent),
		stakes: make(map[uuid.UUID][]user.Stake),
		now:    time.Now,
	}
}

// WithNow makes the popular list tell the time by now: for tests.
func (r *BetEventRepository) WithNow(now func() time.Time) *BetEventRepository {
	r.now = now

	return r
}

func (r *BetEventRepository) Create(ctx context.Context, e *user.BetEvent) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events[e.ID()] = e

	return nil
}

func (r *BetEventRepository) Update(ctx context.Context, e *user.BetEvent) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[e.ID()]; !ok {
		return user.ErrBetEventNotFound
	}
	r.events[e.ID()] = e

	return nil
}

func (r *BetEventRepository) Lock(ctx context.Context, id uuid.UUID) (*user.BetEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.events[id]
	if !ok {
		return nil, user.ErrBetEventNotFound
	}

	return e, nil
}

func (r *BetEventRepository) LockForStake(ctx context.Context, id uuid.UUID) (*user.BetEvent, error) {
	return r.Lock(ctx, id)
}

func (r *BetEventRepository) List(ctx context.Context, list service.BetEventList, viewerID uuid.UUID) ([]service.BetEventView, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.now()
	views := make([]service.BetEventView, 0, len(r.events))
	for _, e := range r.events {
		if list == service.BetEventsPopular && !e.OpenAt(now) {
			continue
		}
		views = append(views, r.view(e, viewerID))
	}

	slices.SortFunc(views, func(a, b service.BetEventView) int {
		if list == service.BetEventsPopular {
			if d := compareInt64(sum(b.Staked), sum(a.Staked)); d != 0 {
				return d
			}
			if d := b.People - a.People; d != 0 {
				return d
			}
		} else if a.Event.State().Over() != b.Event.State().Over() {
			if a.Event.State().Over() {
				return 1
			}

			return -1
		}

		aID, bID := a.Event.ID(), b.Event.ID()

		return bytes.Compare(bID[:], aID[:])
	})

	if len(views) > service.MaxListedBetEvents {
		views = views[:service.MaxListedBetEvents]
	}

	return views, nil
}

func (r *BetEventRepository) view(e *user.BetEvent, viewerID uuid.UUID) service.BetEventView {
	n := len(e.Outcomes())
	v := service.BetEventView{Event: e, Staked: make([]int64, n), Bettors: make([]int, n), Mine: make([]int64, n)}

	people := make(map[uuid.UUID]bool)
	for _, s := range r.stakes[e.ID()] {
		v.Staked[s.Outcome] += s.Amount
		v.Bettors[s.Outcome]++
		people[s.UserID] = true
		if s.UserID == viewerID && viewerID != uuid.Nil {
			v.Mine[s.Outcome] += s.Amount
			v.Payout += s.Payout
		}
	}
	v.People = len(people)

	return v
}

func sum(votes []int64) int64 {
	var total int64
	for _, v := range votes {
		total += v
	}

	return total
}

func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}

	return 0
}

func (r *BetEventRepository) AddStake(ctx context.Context, eventID, userID uuid.UUID, outcome int, amount int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stakes := r.stakes[eventID]
	for i, s := range stakes {
		if s.UserID == userID && s.Outcome == outcome {
			stakes[i].Amount += amount

			return nil
		}
	}
	r.stakes[eventID] = append(stakes, user.Stake{UserID: userID, Outcome: outcome, Amount: amount})

	return nil
}

func (r *BetEventRepository) Stakes(ctx context.Context, eventID uuid.UUID) ([]user.Stake, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.stakes[eventID]), nil
}

func (r *BetEventRepository) SetPayouts(ctx context.Context, eventID uuid.UUID, stakes []user.Stake) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stakes[eventID] = slices.Clone(stakes)

	return nil
}

func (r *BetEventRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[id]; !ok {
		return user.ErrBetEventNotFound
	}
	delete(r.events, id)
	delete(r.stakes, id)

	return nil
}
