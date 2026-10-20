package inmem

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// SetAlert makes UserRepository the labels as well: they are kept with the
// users, as in the table.
func (r *UserRepository) SetAlert(ctx context.Context, userID uuid.UUID, alert user.Alert) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[userID]
	if !ok {
		return user.ErrNotFound
	}

	r.users[userID] = *u.WithAlert(alert)

	return nil
}
