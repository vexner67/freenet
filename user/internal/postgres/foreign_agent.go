package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

const setForeignAgentQuery = `
	UPDATE users
	SET foreign_agent = $2
	WHERE id = $1
`

// SetForeignAgent puts the plaque on the page or takes it; Update never
// writes the column.
func (r *UserRepository) SetForeignAgent(ctx context.Context, userID uuid.UUID, agent bool) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, setForeignAgentQuery, userID, agent)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}
