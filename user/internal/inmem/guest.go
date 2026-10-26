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

// GuestRepository keeps the guests of pages in memory; a deleted user
// takes the visits of both sides, as the foreign keys do.
type GuestRepository struct {
	mu     sync.Mutex
	visits map[uuid.UUID]map[uuid.UUID]time.Time
	seen   map[uuid.UUID]time.Time
}

func NewGuestRepository(users *UserRepository) *GuestRepository {
	r := &GuestRepository{visits: map[uuid.UUID]map[uuid.UUID]time.Time{}, seen: map[uuid.UUID]time.Time{}}
	users.onDelete(r.forget)

	return r
}

func (r *GuestRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.visits, id)
	delete(r.seen, id)
	for _, guests := range r.visits {
		delete(guests, id)
	}
}

func (r *GuestRepository) Visit(ctx context.Context, ownerID, guestID uuid.UUID, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.visits[ownerID] == nil {
		r.visits[ownerID] = map[uuid.UUID]time.Time{}
	}
	r.visits[ownerID][guestID] = at

	return nil
}

// since are the guests of ownerID after since, the last visit first.
func (r *GuestRepository) since(ownerID uuid.UUID, since time.Time) []user.Guest {
	var guests []user.Guest
	for id, at := range r.visits[ownerID] {
		if at.After(since) {
			guests = append(guests, user.Guest{GuestID: id, VisitedAt: at})
		}
	}

	slices.SortFunc(guests, func(a, b user.Guest) int {
		if c := b.VisitedAt.Compare(a.VisitedAt); c != 0 {
			return c
		}

		return compareUUID(b.GuestID, a.GuestID)
	})

	return guests
}

func (r *GuestRepository) Guests(ctx context.Context, ownerID uuid.UUID, since time.Time, after *service.GuestKey, limit int) ([]user.Guest, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var page []user.Guest
	for _, g := range r.since(ownerID, since) {
		if after != nil && (g.VisitedAt.After(after.VisitedAt) ||
			g.VisitedAt.Equal(after.VisitedAt) && compareUUID(g.GuestID, after.GuestID) >= 0) {
			continue
		}
		if len(page) == limit {
			break
		}
		page = append(page, g)
	}

	return page, nil
}

func (r *GuestRepository) CountSince(ctx context.Context, ownerID uuid.UUID, since time.Time, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return min(len(r.since(ownerID, since)), limit), nil
}

func (r *GuestRepository) SeenAt(ctx context.Context, ownerID uuid.UUID) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.seen[ownerID], nil
}

func (r *GuestRepository) MarkSeen(ctx context.Context, ownerID uuid.UUID, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.seen[ownerID] = at

	return nil
}

// compareUUID orders IDs by their bytes, as PostgreSQL does.
func compareUUID(a, b uuid.UUID) int {
	return bytes.Compare(a[:], b[:])
}
