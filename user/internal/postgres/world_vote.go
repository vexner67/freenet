package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// WorldVoteRepository remembers the operations of votes of the world
// service (world_votes).
type WorldVoteRepository struct {
	pool *pgxpool.Pool
}

func NewWorldVoteRepository(pool *pgxpool.Pool) *WorldVoteRepository {
	return &WorldVoteRepository{
		pool: pool,
	}
}

func (r *WorldVoteRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addWorldVoteQuery = `
	INSERT INTO world_votes (
		op_id,
		user_id,
		amount
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (op_id) DO NOTHING
`

func (r *WorldVoteRepository) AddWorldVote(ctx context.Context, opID, userID uuid.UUID, amount int64) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, addWorldVoteQuery, opID, userID, amount)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() == 1, nil
}
