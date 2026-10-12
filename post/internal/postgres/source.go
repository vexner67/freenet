package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// SourceRepository keeps feed_sources: whom each reader reads.
type SourceRepository struct {
	pool *pgxpool.Pool
}

func NewSourceRepository(pool *pgxpool.Pool) *SourceRepository {
	return &SourceRepository{
		pool: pool,
	}
}

func (r *SourceRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addSourceQuery = `
	INSERT INTO feed_sources (
		reader_id,
		source_id,
		club
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (reader_id, source_id) DO UPDATE SET club = EXCLUDED.club
`

func (r *SourceRepository) Add(ctx context.Context, readerID, sourceID uuid.UUID, club bool) error {
	if _, err := r.conn(ctx).Exec(ctx, addSourceQuery, readerID, sourceID, club); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const removeSourceQuery = `DELETE FROM feed_sources WHERE reader_id = $1 AND source_id = $2`

func (r *SourceRepository) Remove(ctx context.Context, readerID, sourceID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, removeSourceQuery, readerID, sourceID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const forgetSourcesQuery = `DELETE FROM feed_sources WHERE reader_id = $1 OR source_id = $1`

func (r *SourceRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, forgetSourcesQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
