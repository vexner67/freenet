package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

type BlockRepository struct {
	pool *pgxpool.Pool
}

func NewBlockRepository(pool *pgxpool.Pool) *BlockRepository {
	return &BlockRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *BlockRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// blockQuery blocks and drops the follows between the two either way in one
// statement, so no follow survives a block; it returns the dropped ones.
const blockQuery = `
	WITH blocked AS (
		INSERT INTO blocks (
			id,
			blocker_id,
			blocked_id
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING
	), dropped AS (
		DELETE FROM follows
		WHERE (follower_id = $2 AND followee_id = $3)
			OR (follower_id = $3 AND followee_id = $2)
		RETURNING
			follower_id,
			followee_id
	)
	SELECT
		follower_id,
		followee_id
	FROM dropped
`

func (r *BlockRepository) Block(ctx context.Context, blockerID, blockedID uuid.UUID) ([]service.Edge, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errs.Wrap("uuid.NewV7", err)
	}

	rows, err := r.conn(ctx).Query(ctx, blockQuery, id, blockerID, blockedID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	dropped, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.Edge, error) {
		var e service.Edge
		err := row.Scan(&e.Follower, &e.Followee)
		return e, err
	})

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		// The user was deleted between the check and the insert.
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return dropped, nil
}

const unblockQuery = `
	DELETE FROM blocks
	WHERE blocker_id = $1
		AND blocked_id = $2
`

func (r *BlockRepository) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, unblockQuery, blockerID, blockedID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const blocksQuery = `
	SELECT
		EXISTS (SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2),
		EXISTS (SELECT 1 FROM blocks WHERE blocker_id = $2 AND blocked_id = $1)
`

func (r *BlockRepository) Blocks(ctx context.Context, a, b uuid.UUID) (aBlockedB, bBlockedA bool, err error) {
	if err = r.conn(ctx).QueryRow(ctx, blocksQuery, a, b).Scan(&aBlockedB, &bBlockedA); err != nil {
		return false, false, errs.Wrap("r.conn.QueryRow", err)
	}

	return aBlockedB, bBlockedA, nil
}

const blockedIDsQuery = `
	SELECT blocked_id
	FROM blocks
	WHERE blocker_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *BlockRepository) BlockedIDs(ctx context.Context, blockerID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, blockedIDsQuery, blockerID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

const listBlockedQuery = `
	SELECT
		b.id,` + qualifiedUserColumns + `
	FROM blocks b
	JOIN users u ON u.id = b.blocked_id
	WHERE b.blocker_id = $1
		AND b.id < $2
	ORDER BY b.id DESC
	LIMIT $3
`

func (r *BlockRepository) ListBlocked(ctx context.Context, blockerID, before uuid.UUID, limit int) ([]service.Blocked, error) {
	if before == uuid.Nil {
		// Above every UUID: start from the newest block.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, listBlockedQuery, blockerID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []service.Blocked
	for rows.Next() {
		var blockID uuid.UUID

		u, err := scanUser(prefixedRow{row: rows, prefix: []any{&blockID}})
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}

		found = append(found, service.Blocked{BlockID: blockID, User: u})
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const countBlockedQuery = `
	SELECT count(*)
	FROM blocks
	WHERE blocker_id = $1
`

func (r *BlockRepository) CountBlocked(ctx context.Context, blockerID uuid.UUID) (int, error) {
	var count int

	if err := r.conn(ctx).QueryRow(ctx, countBlockedQuery, blockerID).Scan(&count); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return count, nil
}
