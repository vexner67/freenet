package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// ReminderRepository finds the events to remind of. It works in the
// transaction of the context, so the mark and the reminders are kept or
// undone together.
type ReminderRepository struct {
	pool *pgxpool.Pool
}

func NewReminderRepository(pool *pgxpool.Pool) *ReminderRepository {
	return &ReminderRepository{
		pool: pool,
	}
}

// takeDueEventsQuery skips the events another replica is taking.
const takeDueEventsQuery = `
	UPDATE clubs
	SET reminded_at = $1
	WHERE id IN (
		SELECT id
		FROM clubs
		WHERE kind = 'event'
			AND reminded_at IS NULL
			AND starts_at > $1
			AND starts_at <= $2
		ORDER BY starts_at
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	)
	RETURNING id
`

func (r *ReminderRepository) TakeDue(ctx context.Context, now, until time.Time, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, takeDueEventsQuery, now, until, limit)
}

const comerIDsQuery = `
	SELECT user_id
	FROM club_members
	WHERE club_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *ReminderRepository) ComerIDs(ctx context.Context, clubID uuid.UUID, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, comerIDsQuery, clubID, limit)
}

func (r *ReminderRepository) ids(ctx context.Context, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := pgpool.Conn(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("pgpool.Conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}
