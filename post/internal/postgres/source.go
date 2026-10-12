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

const forgetSourcesQuery = `
	WITH opened AS (
		DELETE FROM closed_walls WHERE wall_id = $1
	)
	DELETE FROM feed_sources WHERE reader_id = $1 OR source_id = $1
`

func (r *SourceRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, forgetSourcesQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const closeWallQuery = `
	INSERT INTO closed_walls (
		wall_id,
		club
	)
	VALUES ($1, $2)
	ON CONFLICT (wall_id) DO UPDATE SET club = EXCLUDED.club
`

func (r *SourceRepository) Close(ctx context.Context, wallID uuid.UUID, club bool) error {
	if _, err := r.conn(ctx).Exec(ctx, closeWallQuery, wallID, club); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const openWallQuery = `DELETE FROM closed_walls WHERE wall_id = $1`

func (r *SourceRepository) Open(ctx context.Context, wallID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, openWallQuery, wallID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// closedToQuery: $2 is NULL for a viewer who is not signed in.
const closedToQuery = `
	SELECT DISTINCT w
	FROM unnest($1::uuid[]) AS w
	WHERE wall_closed_to(w, $2)
`

func (r *SourceRepository) ClosedTo(ctx context.Context, viewerID uuid.UUID, wallIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(wallIDs) == 0 {
		return nil, nil
	}

	rows, err := r.conn(ctx).Query(ctx, closedToQuery, wallIDs, reader(viewerID))
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	closed := make(map[uuid.UUID]bool)

	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		closed[id] = true
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return closed, nil
}

// reader is a viewer as wall_closed_to takes one: NULL for nobody.
func reader(viewerID uuid.UUID) *uuid.UUID {
	if viewerID == uuid.Nil {
		return nil
	}

	return &viewerID
}
