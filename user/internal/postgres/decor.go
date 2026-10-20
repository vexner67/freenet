package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// DecorRepository keeps the New Year decoration of the site: the one row
// of site_decor.
type DecorRepository struct {
	pool *pgxpool.Pool
}

func NewDecorRepository(pool *pgxpool.Pool) *DecorRepository {
	return &DecorRepository{
		pool: pool,
	}
}

const getDecorQuery = `
	SELECT
		mode
	FROM site_decor
`

func (r *DecorRepository) Decor(ctx context.Context) (user.Decor, error) {
	var mode string

	err := r.pool.QueryRow(ctx, getDecorQuery).Scan(&mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DecorAuto, nil
	}
	if err != nil {
		return "", errs.Wrap("r.pool.QueryRow", err)
	}

	return user.Decor(mode), nil
}

const setDecorQuery = `
	INSERT INTO site_decor (
		mode
	)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET mode = EXCLUDED.mode
`

func (r *DecorRepository) SetDecor(ctx context.Context, decor user.Decor) error {
	if _, err := r.pool.Exec(ctx, setDecorQuery, decor.String()); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
