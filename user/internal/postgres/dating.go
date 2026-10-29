package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// DatingRepository keeps "Знакомства".
type DatingRepository struct {
	pool *pgxpool.Pool
}

func NewDatingRepository(pool *pgxpool.Pool) *DatingRepository {
	return &DatingRepository{pool: pool}
}

func (r *DatingRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const datingCardColumns = `
		user_id,
		goal,
		seek,
		about,
		is_on,
		updated_at
`

func scanDatingCard(row pgx.Row) (user.DatingCard, error) {
	var (
		c          user.DatingCard
		goal, seek string
	)
	if err := row.Scan(&c.UserID, &goal, &seek, &c.About, &c.On, &c.UpdatedAt); err != nil {
		return user.DatingCard{}, err
	}
	c.Goal, c.Seek, c.UpdatedAt = user.DatingGoal(goal), user.Gender(seek), c.UpdatedAt.UTC()

	return c, nil
}

const selectDatingCardQuery = `
	SELECT` + datingCardColumns + `
	FROM dating_cards
	WHERE user_id = $1
`

func (r *DatingRepository) Card(ctx context.Context, userID uuid.UUID) (user.DatingCard, error) {
	c, err := scanDatingCard(r.conn(ctx).QueryRow(ctx, selectDatingCardQuery, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DatingCard{}, user.ErrNotFound
	}

	return c, errs.Wrap("scanDatingCard", err)
}

const saveDatingCardQuery = `
	INSERT INTO dating_cards (` + datingCardColumns + `)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (user_id) DO UPDATE
	SET goal = EXCLUDED.goal,
		seek = EXCLUDED.seek,
		about = EXCLUDED.about,
		is_on = EXCLUDED.is_on,
		updated_at = EXCLUDED.updated_at
`

func (r *DatingRepository) SaveCard(ctx context.Context, c user.DatingCard) error {
	_, err := r.conn(ctx).Exec(ctx, saveDatingCardQuery, c.UserID, string(c.Goal), string(c.Seek), c.About, c.On, c.UpdatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const listDatingCandidatesQuery = `
	SELECT` + datingCardColumns + `
	FROM dating_cards c
	WHERE c.is_on
		AND c.user_id <> $1
		AND NOT EXISTS (
			SELECT 1
			FROM dating_answers a
			WHERE a.from_id = $1
				AND a.to_id = c.user_id
		)
	ORDER BY c.updated_at DESC, c.user_id DESC
	LIMIT $2
`

func (r *DatingRepository) Candidates(ctx context.Context, userID uuid.UUID, limit int) ([]user.DatingCard, error) {
	rows, err := r.conn(ctx).Query(ctx, listDatingCandidatesQuery, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	cards, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.DatingCard, error) {
		return scanDatingCard(row)
	})

	return cards, errs.Wrap("pgx.CollectRows", err)
}

const insertDatingAnswerQuery = `
	INSERT INTO dating_answers (
		from_id,
		to_id,
		is_like,
		answered_at
	)
	VALUES ($1, $2, $3, $4)
`

func (r *DatingRepository) Answer(ctx context.Context, fromID, toID uuid.UUID, like bool, at time.Time) error {
	_, err := r.conn(ctx).Exec(ctx, insertDatingAnswerQuery, fromID, toID, like, at)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolationCode:
			return user.ErrDatingTwice
		case foreignKeyViolationCode:
			return user.ErrNotFound
		}
	}

	return errs.Wrap("r.conn.Exec", err)
}

const selectDatingLikedQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM dating_answers
		WHERE from_id = $1
			AND to_id = $2
			AND is_like
	)
`

func (r *DatingRepository) Liked(ctx context.Context, fromID, toID uuid.UUID) (bool, error) {
	var liked bool
	err := r.conn(ctx).QueryRow(ctx, selectDatingLikedQuery, fromID, toID).Scan(&liked)

	return liked, errs.Wrap("r.conn.QueryRow", err)
}

const listDatingMatchesQuery = `
	SELECT
		mine.to_id,
		GREATEST(mine.answered_at, theirs.answered_at) AS matched_at
	FROM dating_answers mine
	JOIN dating_answers theirs
		ON theirs.from_id = mine.to_id
		AND theirs.to_id = mine.from_id
		AND theirs.is_like
	WHERE mine.from_id = $1
		AND mine.is_like
	ORDER BY matched_at DESC
`

func (r *DatingRepository) Matches(ctx context.Context, userID uuid.UUID) ([]user.DatingMatch, error) {
	rows, err := r.conn(ctx).Query(ctx, listDatingMatchesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.DatingMatch, error) {
		var m user.DatingMatch
		err := row.Scan(&m.UserID, &m.At)
		m.At = m.At.UTC()

		return m, err
	})

	return matches, errs.Wrap("pgx.CollectRows", err)
}
