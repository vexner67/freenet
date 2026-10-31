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
	"github.com/vexner67/freenet/user/internal/user"
)

// CanvasRepository keeps the shared canvas.
type CanvasRepository struct {
	pool *pgxpool.Pool
}

func NewCanvasRepository(pool *pgxpool.Pool) *CanvasRepository {
	return &CanvasRepository{pool: pool}
}

func (r *CanvasRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const canvasCellColumns = `
		x,
		y,
		color,
		user_id,
		painted_at
`

const listCanvasCellsQuery = `
	SELECT` + canvasCellColumns + `
	FROM canvas_cells
`

func (r *CanvasRepository) Cells(ctx context.Context) ([]user.CanvasCell, error) {
	rows, err := r.conn(ctx).Query(ctx, listCanvasCellsQuery)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	cells, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.CanvasCell, error) {
		return scanCanvasCell(row)
	})

	return cells, errs.Wrap("pgx.CollectRows", err)
}

func scanCanvasCell(row pgx.Row) (user.CanvasCell, error) {
	var (
		c      user.CanvasCell
		userID *uuid.UUID
	)
	if err := row.Scan(&c.X, &c.Y, &c.Color, &userID, &c.At); err != nil {
		return user.CanvasCell{}, err
	}
	if userID != nil {
		c.UserID = *userID
	}
	c.At = c.At.UTC()

	return c, nil
}

const selectCanvasCellQuery = `
	SELECT` + canvasCellColumns + `
	FROM canvas_cells
	WHERE x = $1
		AND y = $2
`

func (r *CanvasRepository) Cell(ctx context.Context, x, y int) (user.CanvasCell, error) {
	c, err := scanCanvasCell(r.conn(ctx).QueryRow(ctx, selectCanvasCellQuery, x, y))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.CanvasCell{}, user.ErrNotFound
	}

	return c, errs.Wrap("scanCanvasCell", err)
}

const paintCanvasCellQuery = `
	INSERT INTO canvas_cells (` + canvasCellColumns + `)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (x, y) DO UPDATE
	SET color = EXCLUDED.color,
		user_id = EXCLUDED.user_id,
		painted_at = EXCLUDED.painted_at
`

func (r *CanvasRepository) PutCell(ctx context.Context, c user.CanvasCell) error {
	_, err := r.conn(ctx).Exec(ctx, paintCanvasCellQuery, c.X, c.Y, c.Color, c.UserID, c.At)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}
