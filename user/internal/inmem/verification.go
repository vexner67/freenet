package inmem

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// SetVerified makes UserRepository the checkmarks as well: they are kept
// with the users, as in the table.
func (r *UserRepository) SetVerified(ctx context.Context, userID uuid.UUID, verified bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[userID]
	if !ok {
		return user.ErrNotFound
	}

	r.users[userID] = *u.WithVerified(verified)

	return nil
}
