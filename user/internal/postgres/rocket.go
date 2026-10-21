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

// RocketRepository keeps the rounds of "Ракетка": the table rocket_rounds.
type RocketRepository struct {
	pool *pgxpool.Pool
}

func NewRocketRepository(pool *pgxpool.Pool) *RocketRepository {
	return &RocketRepository{
		pool: pool,
	}
}

func (r *RocketRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveRocketRoundQuery = `
	INSERT INTO rocket_rounds (
		id,
		user_id,
		amount,
		crash,
		auto,
		started_at,
		cashed,
		payout
	)
	VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6, NULLIF($7, 0), $8)
`

func (r *RocketRepository) Save(ctx context.Context, round *user.RocketRound) error {
	_, err := r.conn(ctx).Exec(ctx, saveRocketRoundQuery,
		round.ID(),
		round.UserID(),
		round.Amount(),
		round.Crash(),
		round.Auto(),
		round.StartedAt(),
		round.Cashed(),
		round.Payout(),
	)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getRocketRoundQuery = `
	SELECT
		id,
		user_id,
		amount,
		crash,
		COALESCE(auto, 0),
		started_at,
		COALESCE(cashed, 0),
		payout
	FROM rocket_rounds
	WHERE id = $1
`

func (r *RocketRepository) Get(ctx context.Context, id uuid.UUID) (*user.RocketRound, error) {
	round, err := r.get(ctx, getRocketRoundQuery, id)

	return round, errs.Wrap("r.get", err)
}

const lockRocketRoundQuery = getRocketRoundQuery + `
	FOR UPDATE
`

func (r *RocketRepository) Lock(ctx context.Context, id uuid.UUID) (*user.RocketRound, error) {
	round, err := r.get(ctx, lockRocketRoundQuery, id)

	return round, errs.Wrap("r.get", err)
}

func (r *RocketRepository) get(ctx context.Context, query string, id uuid.UUID) (*user.RocketRound, error) {
	var (
		roundID, userID uuid.UUID
		amount, payout  int64
		crash, auto     int
		cashed          int
		startedAt       time.Time
	)

	err := r.conn(ctx).QueryRow(ctx, query, id).Scan(&roundID, &userID, &amount, &crash, &auto, &startedAt, &cashed, &payout)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrRocketNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	return user.RestoreRocketRound(roundID, userID, amount, crash, auto, startedAt, cashed, payout), nil
}

const setRocketRoundCashedQuery = `
	UPDATE rocket_rounds
	SET cashed = $2,
		payout = $3
	WHERE id = $1
`

func (r *RocketRepository) SetCashed(ctx context.Context, round *user.RocketRound) error {
	tag, err := r.conn(ctx).Exec(ctx, setRocketRoundCashedQuery, round.ID(), round.Cashed(), round.Payout())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrRocketNotFound
	}

	return nil
}

const deleteRocketRoundsBeforeQuery = `
	DELETE FROM rocket_rounds
	WHERE user_id = $1
		AND started_at < $2
`

func (r *RocketRepository) DeleteBefore(ctx context.Context, userID uuid.UUID, before time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteRocketRoundsBeforeQuery, userID, before); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
