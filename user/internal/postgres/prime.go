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
	"github.com/vexner67/freenet/user/internal/user"
)

// PrimeRepository keeps Freenet Prime: the subscriptions in the users
// table and the prices in the one row of prime_prices.
type PrimeRepository struct {
	pool *pgxpool.Pool
}

func NewPrimeRepository(pool *pgxpool.Pool) *PrimeRepository {
	return &PrimeRepository{
		pool: pool,
	}
}

func (r *PrimeRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getPrimePricesQuery = `
	SELECT
		month,
		quarter,
		year
	FROM prime_prices
`

func (r *PrimeRepository) Prices(ctx context.Context) (user.PrimePrices, error) {
	var p user.PrimePrices

	err := r.conn(ctx).QueryRow(ctx, getPrimePricesQuery).Scan(&p.Month, &p.Quarter, &p.Year)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultPrimePrices, nil
	}
	if err != nil {
		return user.PrimePrices{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return p, nil
}

const setPrimePricesQuery = `
	INSERT INTO prime_prices (
		month,
		quarter,
		year
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (only_row) DO UPDATE
	SET month = EXCLUDED.month,
		quarter = EXCLUDED.quarter,
		year = EXCLUDED.year
`

func (r *PrimeRepository) SetPrices(ctx context.Context, p user.PrimePrices) error {
	if _, err := r.conn(ctx).Exec(ctx, setPrimePricesQuery, p.Month, p.Quarter, p.Year); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// extendPrimeQuery adds the days of a period to the end of the
// subscription, or to now when it has ended, as user.Prime.Extended does;
// two purchases at once add both.
const extendPrimeQuery = `
	UPDATE users
	SET prime_until = greatest(coalesce(prime_until, $3), $3) + make_interval(days => $2)
	WHERE id = $1
	RETURNING prime_until
`

func (r *PrimeRepository) Extend(ctx context.Context, userID uuid.UUID, period user.PrimePeriod, now time.Time) (time.Time, error) {
	var until time.Time

	err := r.conn(ctx).QueryRow(ctx, extendPrimeQuery, userID, period.Days(), now).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, user.ErrNotFound
	}
	if err != nil {
		return time.Time{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return until.UTC(), nil
}

const setPrimeInvisibleQuery = `
	UPDATE users
	SET prime_invisible = $2
	WHERE id = $1
`

func (r *PrimeRepository) SetInvisible(ctx context.Context, userID uuid.UUID, invisible bool) error {
	tag, err := r.conn(ctx).Exec(ctx, setPrimeInvisibleQuery, userID, invisible)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}
