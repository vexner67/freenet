package inmem

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// IgnoreRepository keeps the ignored sources of every user, oldest first.
type IgnoreRepository struct {
	mu      sync.RWMutex
	ignored map[uuid.UUID][]uuid.UUID
}

func NewIgnoreRepository(users *UserRepository) *IgnoreRepository {
	r := &IgnoreRepository{ignored: make(map[uuid.UUID][]uuid.UUID)}
	users.onDelete(r.forget)

	return r
}

// forget drops what a deleted user ignored.
func (r *IgnoreRepository) forget(userID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.ignored, userID)
}

func (r *IgnoreRepository) Ignore(ctx context.Context, userID, sourceID uuid.UUID, limit int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	had := r.ignored[userID]
	if slices.Contains(had, sourceID) {
		return true, nil
	}

	if len(had) >= limit {
		return false, nil
	}

	r.ignored[userID] = append(had, sourceID)

	return true, nil
}

func (r *IgnoreRepository) Unignore(ctx context.Context, userID uuid.UUID, sourceIDs []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ignored[userID] = slices.DeleteFunc(r.ignored[userID], func(id uuid.UUID) bool {
		return slices.Contains(sourceIDs, id)
	})

	return nil
}

func (r *IgnoreRepository) Ignored(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := slices.Clone(r.ignored[userID])
	slices.Reverse(ids)

	if len(ids) > limit {
		ids = ids[:limit]
	}

	return ids, nil
}
