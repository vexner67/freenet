package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// TopRepository counts the boards of "Рейтинги": from users, the
// statement and the pets.
type TopRepository struct {
	pool *pgxpool.Pool
}

func NewTopRepository(pool *pgxpool.Pool) *TopRepository {
	return &TopRepository{
		pool: pool,
	}
}

func (r *TopRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// shownUser is the condition of a user u neither deleted nor banned at
// $1.
const shownUser = `u.deactivated_at IS NULL AND (u.banned_at IS NULL OR u.banned_until <= $1)`

const richTopQuery = `
	SELECT
		u.id,
		u.votes
	FROM users AS u
	WHERE u.votes > 0
		AND ` + shownUser + `
	ORDER BY u.votes DESC, u.id
	LIMIT $2
`

func (r *TopRepository) Rich(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error) {
	entries, err := r.entries(ctx, richTopQuery, now, limit)

	return entries, errs.Wrap("r.entries", err)
}

const earnedTopQuery = `
	SELECT
		o.user_id,
		sum(o.amount)::bigint AS earned
	FROM vote_operations AS o
	JOIN users AS u ON u.id = o.user_id
	WHERE o.kind = ANY ($3)
		AND o.id >= $4
		AND ` + shownUser + `
	GROUP BY o.user_id
	HAVING sum(o.amount) > 0
	ORDER BY earned DESC, o.user_id
	LIMIT $2
`

func (r *TopRepository) Earned(ctx context.Context, kinds []user.VoteKind, since, now time.Time, limit int) ([]user.TopEntry, error) {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}

	entries, err := r.entries(ctx, earnedTopQuery, now, limit, names, user.OperationsSince(since))

	return entries, errs.Wrap("r.entries", err)
}

func (r *TopRepository) entries(ctx context.Context, query string, args ...any) ([]user.TopEntry, error) {
	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.TopEntry, error) {
		var e user.TopEntry
		err := row.Scan(&e.UserID, &e.Value)

		return e, err
	})

	return entries, errs.Wrap("pgx.CollectRows", err)
}

const petsTopQuery = `
	SELECT
		p.user_id,
		p.rating,
		p.kind,
		p.name
	FROM pets AS p
	JOIN users AS u ON u.id = p.user_id
	WHERE ` + shownUser + `
	ORDER BY p.rating DESC, p.id
	LIMIT $2
`

func (r *TopRepository) Pets(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error) {
	rows, err := r.conn(ctx).Query(ctx, petsTopQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.TopEntry, error) {
		var (
			e          user.TopEntry
			kind, name string
		)
		err := row.Scan(&e.UserID, &e.Value, &kind, &name)
		e.PetKind, e.PetName = user.PetKind(kind), user.PetName(name)

		return e, err
	})

	return entries, errs.Wrap("pgx.CollectRows", err)
}
