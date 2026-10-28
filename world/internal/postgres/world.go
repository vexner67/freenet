// Package postgres keeps the worlds of «Мир» in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/world/internal/world"
)

// WorldRepository keeps the worlds (worlds).
type WorldRepository struct {
	pool *pgxpool.Pool
}

func NewWorldRepository(pool *pgxpool.Pool) *WorldRepository {
	return &WorldRepository{
		pool: pool,
	}
}

func (r *WorldRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// nullTime is a zero time as NULL.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

const saveWorldQuery = `
	INSERT INTO worlds (
		id,
		seed,
		grid,
		status,
		created_at,
		opened_at,
		ended_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (id) DO UPDATE SET
		status = EXCLUDED.status,
		opened_at = EXCLUDED.opened_at,
		ended_at = EXCLUDED.ended_at
`

func (r *WorldRepository) SaveWorld(ctx context.Context, w *world.World) error {
	_, err := r.conn(ctx).Exec(ctx, saveWorldQuery,
		w.ID(), int64(w.Seed()), w.Grid(), string(w.Status()), w.CreatedAt(), nullTime(w.OpenedAt()), nullTime(w.EndedAt()))

	return errs.Wrap("r.conn.Exec", err)
}

const worldColumns = `
		id,
		seed,
		grid,
		status,
		created_at,
		opened_at,
		ended_at
`

func scanWorld(row pgx.Row) (*world.World, error) {
	var (
		s                world.Stored
		seed             int64
		status           string
		opened, finished *time.Time
	)

	if err := row.Scan(&s.ID, &seed, &s.Grid, &status, &s.CreatedAt, &opened, &finished); err != nil {
		return nil, err
	}

	s.Seed, s.Status = uint64(seed), world.Status(status)
	if opened != nil {
		s.OpenedAt = *opened
	}
	if finished != nil {
		s.EndedAt = *finished
	}

	return world.Restore(s), nil
}

const getWorldQuery = `SELECT` + worldColumns + `FROM worlds WHERE id = $1`

func (r *WorldRepository) World(ctx context.Context, id uuid.UUID) (*world.World, error) {
	w, err := scanWorld(r.conn(ctx).QueryRow(ctx, getWorldQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNotFound
	}

	return w, errs.Wrap("scanWorld", err)
}

const openWorldQuery = `SELECT` + worldColumns + `FROM worlds WHERE status = 'open'`

func (r *WorldRepository) OpenWorld(ctx context.Context) (*world.World, error) {
	w, err := scanWorld(r.conn(ctx).QueryRow(ctx, openWorldQuery))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNoWorld
	}

	return w, errs.Wrap("scanWorld", err)
}

const listWorldsQuery = `SELECT` + worldColumns + `FROM worlds ORDER BY id DESC LIMIT $1`

func (r *WorldRepository) Worlds(ctx context.Context, limit int) ([]*world.World, error) {
	rows, err := r.conn(ctx).Query(ctx, listWorldsQuery, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ws, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*world.World, error) { return scanWorld(row) })

	return ws, errs.Wrap("pgx.CollectRows", err)
}

// claimGenerationQuery takes the oldest world waiting for its map that
// no replica makes; SKIP LOCKED keeps two replicas off the same one.
const claimGenerationQuery = `
	UPDATE worlds
	SET lease_until = $1
	WHERE id = (
		SELECT id
		FROM worlds
		WHERE status = 'generating'
			AND (lease_until IS NULL OR lease_until <= $2)
		ORDER BY id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	)
	RETURNING` + worldColumns

func (r *WorldRepository) ClaimGeneration(ctx context.Context, now time.Time, lease time.Duration) (*world.World, error) {
	w, err := scanWorld(r.conn(ctx).QueryRow(ctx, claimGenerationQuery, now.Add(lease), now))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNotFound
	}

	return w, errs.Wrap("scanWorld", err)
}
