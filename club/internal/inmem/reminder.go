package inmem

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/pkg/errs"
)

// ReminderRepository finds the events to remind of, as in PostgreSQL.
type ReminderRepository struct {
	s *Store
}

func (r *ReminderRepository) TakeDue(ctx context.Context, now, until time.Time, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var due []club.Stored
	for id, c := range r.s.clubs {
		start := c.Schedule.Start
		if c.Kind == club.KindEvent && !r.s.reminded[id] && start.After(now) && !start.After(until) {
			due = append(due, c)
		}
	}

	slices.SortFunc(due, func(a, b club.Stored) int {
		return cmp.Or(a.Schedule.Start.Compare(b.Schedule.Start), cmp.Compare(a.ID.String(), b.ID.String()))
	})

	ids := make([]uuid.UUID, 0, min(len(due), limit))
	for _, c := range due[:min(len(due), limit)] {
		r.s.reminded[c.ID] = true
		ids = append(ids, c.ID)
	}

	return ids, nil
}

func (r *ReminderRepository) ComerIDs(ctx context.Context, clubID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	found := r.s.sorted(func(m *member) bool { return m.clubID == clubID }, uuid.Nil, limit)

	ids := make([]uuid.UUID, len(found))
	for i, id := range found {
		ids[i] = r.s.members[id].userID
	}

	return ids, nil
}
