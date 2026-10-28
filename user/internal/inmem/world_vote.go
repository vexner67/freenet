package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// WorldVoteRepository remembers in memory the operations of votes of the
// world service.
type WorldVoteRepository struct {
	mu  sync.Mutex
	ops map[uuid.UUID]struct{}
}

func NewWorldVoteRepository() *WorldVoteRepository {
	return &WorldVoteRepository{
		ops: make(map[uuid.UUID]struct{}),
	}
}

func (r *WorldVoteRepository) AddWorldVote(ctx context.Context, opID, _ uuid.UUID, _ int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.ops[opID]; ok {
		return false, nil
	}

	r.ops[opID] = struct{}{}

	return true, nil
}
