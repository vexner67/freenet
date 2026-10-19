package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// VoteRateRepository keeps the rate of a vote: the one row of vote_rates.
type VoteRateRepository struct {
	pool *pgxpool.Pool
}

func NewVoteRateRepository(pool *pgxpool.Pool) *VoteRateRepository {
	return &VoteRateRepository{
		pool: pool,
	}
}

const getVoteRateQuery = `
	SELECT
		kopecks
	FROM vote_rates
`

func (r *VoteRateRepository) Rate(ctx context.Context) (user.VoteRate, error) {
	var kopecks int64

	err := r.pool.QueryRow(ctx, getVoteRateQuery).Scan(&kopecks)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultVoteRate, nil
	}
	if err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return user.VoteRate(kopecks), nil
}

const setVoteRateQuery = `
	INSERT INTO vote_rates (
		kopecks
	)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET kopecks = EXCLUDED.kopecks
`

func (r *VoteRateRepository) SetRate(ctx context.Context, rate user.VoteRate) error {
	if _, err := r.pool.Exec(ctx, setVoteRateQuery, rate.Kopecks()); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
