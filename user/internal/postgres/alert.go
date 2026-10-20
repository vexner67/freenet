package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

const setAlertQuery = `
	UPDATE users
	SET alert = NULLIF($2, '')
	WHERE id = $1
`

// SetAlert puts the label on the page or, with "", takes it; Update never
// writes the column.
func (r *UserRepository) SetAlert(ctx context.Context, userID uuid.UUID, alert user.Alert) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, setAlertQuery, userID, alert.String())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}
