package inmem

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// RequestRepository keeps the requests to join the clubs of its Store.
type RequestRepository struct {
	s *Store
}

// requested finds the request of userID to join clubID; the caller holds
// a lock.
func (s *Store) requested(clubID, userID uuid.UUID) (uuid.UUID, bool) {
	for id, r := range s.requests {
		if r.clubID == clubID && r.userID == userID {
			return id, true
		}
	}

	return uuid.Nil, false
}

func (r *RequestRepository) Add(ctx context.Context, clubID, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.clubs[clubID]; !ok {
		return club.ErrNotFound
	}

	if _, ok := r.s.requested(clubID, userID); ok {
		return nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	r.s.requests[id] = request{clubID: clubID, userID: userID}

	return nil
}

func (r *RequestRepository) Delete(ctx context.Context, clubID, userID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	id, ok := r.s.requested(clubID, userID)
	delete(r.s.requests, id)

	return ok, nil
}

func (r *RequestRepository) Has(ctx context.Context, clubID, userID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	_, ok := r.s.requested(clubID, userID)

	return ok, nil
}

func (r *RequestRepository) List(ctx context.Context, clubID, before uuid.UUID, limit int) ([]service.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []service.Request
	for id, req := range r.s.requests {
		if req.clubID == clubID && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			found = append(found, service.Request{ID: id, UserID: req.userID})
		}
	}

	// UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b service.Request) int { return bytes.Compare(b.ID[:], a.ID[:]) })

	return found[:min(limit, len(found))], nil
}

func (r *RequestRepository) Count(ctx context.Context, clubID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	n := 0
	for _, req := range r.s.requests {
		if req.clubID == clubID {
			n++
		}
	}

	return n, nil
}

func (r *RequestRepository) DeleteByClub(ctx context.Context, clubID uuid.UUID) error {
	return r.deleteWhere(ctx, func(req request) bool { return req.clubID == clubID })
}

func (r *RequestRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	return r.deleteWhere(ctx, func(req request) bool { return req.userID == userID })
}

func (r *RequestRepository) deleteWhere(ctx context.Context, match func(request) bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for id, req := range r.s.requests {
		if match(req) {
			delete(r.s.requests, id)
		}
	}

	return nil
}
