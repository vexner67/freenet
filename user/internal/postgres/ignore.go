package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

type IgnoreRepository struct {
	pool *pgxpool.Pool
}

func NewIgnoreRepository(pool *pgxpool.Pool) *IgnoreRepository {
	return &IgnoreRepository{pool: pool}
}

func (r *IgnoreRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// lockIgnoringUserQuery makes those who ignore sources for one user take
// turns: the count below has to see what the others have added, and a
// statement that waited for a lock still counts what was there when it
// began.
const lockIgnoringUserQuery = `
	SELECT id
	FROM users
	WHERE id = $1
	FOR UPDATE
`

// ignoreQuery counts and adds in one statement, under the lock of the
// user's row. It tells whether the source is ignored now.
const ignoreQuery = `
	WITH had AS (
		SELECT
			count(*) AS total,
			coalesce(bool_or(source_id = $3), false) AS already
		FROM feed_ignores
		WHERE user_id = $2
	), added AS (
		INSERT INTO feed_ignores (
			id,
			user_id,
			source_id
		)
		SELECT
			$1,
			$2,
			$3
		FROM had
		WHERE NOT had.already
			AND had.total < $4
		ON CONFLICT (user_id, source_id) DO NOTHING
	)
	SELECT had.already OR had.total < $4
	FROM had
`

func (r *IgnoreRepository) Ignore(ctx context.Context, userID, sourceID uuid.UUID, limit int) (bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return false, errs.Wrap("uuid.NewV7", err)
	}

	var ignored bool

	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, lockIgnoringUserQuery, userID).Scan(&locked); err != nil {
			return errs.Wrap("tx.QueryRow", err)
		}

		return errs.Wrap("tx.QueryRow", tx.QueryRow(ctx, ignoreQuery, id, userID, sourceID, limit).Scan(&ignored))
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, user.ErrNotFound
	}
	if err != nil {
		return false, errs.Wrap("pgx.BeginFunc", err)
	}

	return ignored, nil
}

const unignoreQuery = `
	DELETE FROM feed_ignores
	WHERE user_id = $1
		AND source_id = ANY($2)
`

func (r *IgnoreRepository) Unignore(ctx context.Context, userID uuid.UUID, sourceIDs []uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, unignoreQuery, userID, sourceIDs); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const ignoredQuery = `
	SELECT source_id
	FROM feed_ignores
	WHERE user_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *IgnoreRepository) Ignored(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, ignoredQuery, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}
