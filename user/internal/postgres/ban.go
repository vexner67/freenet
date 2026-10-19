package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

const setBanQuery = `
	UPDATE users
	SET banned_at = $2,
		banned_until = $3,
		ban_reason = $4
	WHERE id = $1
`

// SetBan blocks the page or, with the zero ban, lets it be; Update never
// writes these columns.
func (r *UserRepository) SetBan(ctx context.Context, userID uuid.UUID, ban user.Ban) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, setBanQuery,
		userID, timeToDB(ban.At), timeToDB(ban.Until), ban.Reason.String())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}
