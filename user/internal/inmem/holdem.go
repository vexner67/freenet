package inmem

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// HoldemRepository keeps the tables of "Покер" in memory, as copies: a
// table read is changed apart from the one kept until saved.
type HoldemRepository struct {
	mu     sync.Mutex
	tables map[uuid.UUID]user.HoldemState
}

func NewHoldemRepository() *HoldemRepository {
	return &HoldemRepository{tables: make(map[uuid.UUID]user.HoldemState)}
}

// copyHoldem is a state that shares no slice with s.
func copyHoldem(s user.HoldemState) user.HoldemState {
	s.Players = slices.Clone(s.Players)
	for i := range s.Players {
		s.Players[i].Cards = slices.Clone(s.Players[i].Cards)
	}
	s.Invited = slices.Clone(s.Invited)
	s.Deck = slices.Clone(s.Deck)
	s.Board = slices.Clone(s.Board)

	return s
}

func (r *HoldemRepository) AddTable(ctx context.Context, t *user.HoldemTable) error {
	return r.SaveTable(ctx, t)
}

func (r *HoldemRepository) Table(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.tables[id]
	if !ok {
		return nil, user.ErrTableNotFound
	}

	return user.RestoreHoldemTable(id, copyHoldem(s)), nil
}

// LockTable is Table: there are no transactions in memory.
func (r *HoldemRepository) LockTable(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error) {
	return r.Table(ctx, id)
}

func (r *HoldemRepository) SaveTable(ctx context.Context, t *user.HoldemTable) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tables[t.ID()] = copyHoldem(t.State())

	return nil
}

func (r *HoldemRepository) DeleteTable(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tables, id)

	return nil
}

// all are the tables kept, restored, newest first.
func (r *HoldemRepository) all() []*user.HoldemTable {
	tables := make([]*user.HoldemTable, 0, len(r.tables))
	for id, s := range r.tables {
		tables = append(tables, user.RestoreHoldemTable(id, copyHoldem(s)))
	}
	slices.SortFunc(tables, func(a, b *user.HoldemTable) int { return b.CreatedAt().Compare(a.CreatedAt()) })

	return tables
}

func (r *HoldemRepository) LockFree(ctx context.Context, bigBlind int64, userID uuid.UUID) (*user.HoldemTable, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var found *user.HoldemTable
	for _, t := range r.all() {
		if t.Private() || t.BigBlind() != bigBlind || t.Players() >= len(t.Seats()) || t.SeatOf(userID) >= 0 {
			continue
		}
		if found == nil || t.Players() > found.Players() {
			found = t
		}
	}
	if found == nil {
		return nil, user.ErrTableNotFound
	}

	return found, nil
}

func (r *HoldemRepository) Tables(ctx context.Context, section service.TableSection, userID uuid.UUID) ([]*user.HoldemTable, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var tables []*user.HoldemTable
	for _, t := range r.all() {
		seated := t.SeatOf(userID) >= 0

		switch section {
		case service.TablesPublic:
			if t.Private() {
				continue
			}
		case service.TablesMine:
			if !seated {
				continue
			}
		case service.TablesInvited:
			if seated || !slices.Contains(t.Invited(), userID) {
				continue
			}
		default:
			return nil, service.ErrTableSection
		}
		tables = append(tables, t)
	}

	if section == service.TablesPublic {
		slices.SortStableFunc(tables, func(a, b *user.HoldemTable) int { return b.Players() - a.Players() })
	}

	return tables[:min(len(tables), service.MaxListedTables)], nil
}

func (r *HoldemRepository) Due(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var ids []uuid.UUID
	for _, t := range r.all() {
		if due := t.Due(); !due.IsZero() && !due.After(now) && len(ids) < limit {
			ids = append(ids, t.ID())
		}
	}

	return ids, nil
}
