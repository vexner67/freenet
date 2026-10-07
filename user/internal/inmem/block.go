package inmem

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// BlockRepository keeps blocks with the follows of a FollowRepository,
// under its lock: a block drops the follows in the same step, as the one
// statement does in PostgreSQL.
type BlockRepository struct {
	follows *FollowRepository
	blocks  map[edge]uuid.UUID // block ID by (blocker, blocked)
}

func NewBlockRepository(follows *FollowRepository) *BlockRepository {
	r := &BlockRepository{
		follows: follows,
		blocks:  make(map[edge]uuid.UUID),
	}
	follows.users.onDelete(r.forget)

	return r
}

// forget drops the blocks of a deleted user, both ways.
func (r *BlockRepository) forget(userID uuid.UUID) {
	r.follows.mu.Lock()
	defer r.follows.mu.Unlock()

	for e := range r.blocks {
		if e.follower == userID || e.followee == userID {
			delete(r.blocks, e)
		}
	}
}

func (r *BlockRepository) Block(ctx context.Context, blockerID, blockedID uuid.UUID) ([]service.Edge, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, errs.Wrap("uuid.NewV7", err)
	}

	r.follows.mu.Lock()
	defer r.follows.mu.Unlock()

	if _, ok := r.blocks[edge{blockerID, blockedID}]; !ok {
		r.blocks[edge{blockerID, blockedID}] = id
	}

	var dropped []service.Edge
	for _, e := range []edge{{blockerID, blockedID}, {blockedID, blockerID}} {
		if _, ok := r.follows.follows[e]; ok {
			delete(r.follows.follows, e)
			dropped = append(dropped, service.Edge{Follower: e.follower, Followee: e.followee})
		}
	}

	return dropped, nil
}

func (r *BlockRepository) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.follows.mu.Lock()
	defer r.follows.mu.Unlock()

	delete(r.blocks, edge{blockerID, blockedID})

	return nil
}

func (r *BlockRepository) Blocks(ctx context.Context, a, b uuid.UUID) (aBlockedB, bBlockedA bool, err error) {
	if err = ctx.Err(); err != nil {
		return false, false, errs.Wrap("ctx.Err", err)
	}

	r.follows.mu.RLock()
	defer r.follows.mu.RUnlock()

	_, aBlockedB = r.blocks[edge{a, b}]
	_, bBlockedA = r.blocks[edge{b, a}]

	return aBlockedB, bBlockedA, nil
}

func (r *BlockRepository) ListBlocked(ctx context.Context, blockerID, before uuid.UUID, limit int) ([]service.Blocked, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	type hit struct {
		blockID, blocked uuid.UUID
	}

	r.follows.mu.RLock()

	var hits []hit
	for e, id := range r.blocks {
		if e.follower == blockerID && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			hits = append(hits, hit{id, e.followee})
		}
	}

	r.follows.mu.RUnlock()

	slices.SortFunc(hits, func(a, b hit) int { return bytes.Compare(b.blockID[:], a.blockID[:]) })

	found := make([]service.Blocked, 0, min(len(hits), limit))
	for _, h := range hits {
		if len(found) == limit {
			break
		}

		u, err := r.follows.users.GetByID(ctx, h.blocked)
		if err != nil && !errors.Is(err, user.ErrNotFound) {
			return nil, errs.Wrap("r.follows.users.GetByID", err)
		}
		if u != nil {
			found = append(found, service.Blocked{BlockID: h.blockID, User: u})
		}
	}

	return found, nil
}

func (r *BlockRepository) CountBlocked(ctx context.Context, blockerID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.follows.mu.RLock()
	defer r.follows.mu.RUnlock()

	count := 0
	for e := range r.blocks {
		if e.follower == blockerID {
			count++
		}
	}

	return count, nil
}
