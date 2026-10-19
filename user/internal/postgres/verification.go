package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

const setVerifiedQuery = `
	UPDATE users
	SET verified = $2
	WHERE id = $1
`

// SetVerified gives the page its checkmark or takes it; Update never
// writes the column.
func (r *UserRepository) SetVerified(ctx context.Context, userID uuid.UUID, verified bool) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, setVerifiedQuery, userID, verified)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}
