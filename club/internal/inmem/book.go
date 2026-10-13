package inmem

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// BookRepository reads the memberships by ID, as in PostgreSQL.
type BookRepository struct {
	s *Store
}

func (s *Store) Book() *BookRepository {
	return &BookRepository{s: s}
}

func (r *BookRepository) ClubIDs(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	ids := make([]uuid.UUID, 0, len(r.s.clubs))
	for id := range r.s.clubs {
		ids = append(ids, id)
	}

	return above(ids, after, limit), nil
}

func (r *BookRepository) MemberIDs(ctx context.Context, clubID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var ids []uuid.UUID
	for _, m := range r.s.members {
		if m.clubID == clubID {
			ids = append(ids, m.userID)
		}
	}

	return above(ids, after, limit), nil
}

// LockClub: there are no transactions in memory to hold a lock.
func (r *BookRepository) LockClub(ctx context.Context, _ uuid.UUID) error {
	return errs.Wrap("ctx.Err", ctx.Err())
}

// above are up to limit of ids above after, ascending.
func above(ids []uuid.UUID, after uuid.UUID, limit int) []uuid.UUID {
	ids = slices.DeleteFunc(ids, func(id uuid.UUID) bool {
		return bytes.Compare(id[:], after[:]) <= 0
	})

	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	})

	return ids[:min(limit, len(ids))]
}
