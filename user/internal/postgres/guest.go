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
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// GuestRepository keeps the guests of pages in page_visits and when their
// owners last looked at them in guest_marks.
type GuestRepository struct {
	pool *pgxpool.Pool
}

func NewGuestRepository(pool *pgxpool.Pool) *GuestRepository {
	return &GuestRepository{
		pool: pool,
	}
}

func (r *GuestRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const visitPageQuery = `
	INSERT INTO page_visits (
		owner_id,
		guest_id,
		visited_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (owner_id, guest_id) DO UPDATE
	SET visited_at = GREATEST(page_visits.visited_at, EXCLUDED.visited_at)
`

func (r *GuestRepository) Visit(ctx context.Context, ownerID, guestID uuid.UUID, at time.Time) error {
	_, err := r.conn(ctx).Exec(ctx, visitPageQuery, ownerID, guestID, at)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const listGuestsQuery = `
	SELECT
		guest_id,
		visited_at
	FROM page_visits
	WHERE owner_id = $1
		AND visited_at > $2
		AND ($3::timestamptz IS NULL OR (visited_at, guest_id) < ($3, $4))
	ORDER BY visited_at DESC, guest_id DESC
	LIMIT $5
`

func (r *GuestRepository) Guests(ctx context.Context, ownerID uuid.UUID, since time.Time, after *service.GuestKey, limit int) ([]user.Guest, error) {
	var (
		afterAt *time.Time
		afterID uuid.UUID
	)
	if after != nil {
		afterAt, afterID = &after.VisitedAt, after.GuestID
	}

	rows, err := r.conn(ctx).Query(ctx, listGuestsQuery, ownerID, since, afterAt, afterID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	guests, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.Guest, error) {
		var g user.Guest
		err := row.Scan(&g.GuestID, &g.VisitedAt)

		return g, err
	})

	return guests, errs.Wrap("pgx.CollectRows", err)
}

const countGuestsQuery = `
	SELECT count(*)
	FROM (
		SELECT 1
		FROM page_visits
		WHERE owner_id = $1
			AND visited_at > $2
		LIMIT $3
	) AS guests
`

func (r *GuestRepository) CountSince(ctx context.Context, ownerID uuid.UUID, since time.Time, limit int) (int, error) {
	var n int
	err := r.conn(ctx).QueryRow(ctx, countGuestsQuery, ownerID, since, limit).Scan(&n)

	return n, errs.Wrap("r.conn.QueryRow", err)
}

const guestsSeenQuery = `
	SELECT seen_at
	FROM guest_marks
	WHERE owner_id = $1
`

func (r *GuestRepository) SeenAt(ctx context.Context, ownerID uuid.UUID) (time.Time, error) {
	var at time.Time

	err := r.conn(ctx).QueryRow(ctx, guestsSeenQuery, ownerID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}

	return at, errs.Wrap("r.conn.QueryRow", err)
}

const markGuestsSeenQuery = `
	INSERT INTO guest_marks (
		owner_id,
		seen_at
	)
	VALUES ($1, $2)
	ON CONFLICT (owner_id) DO UPDATE
	SET seen_at = GREATEST(guest_marks.seen_at, EXCLUDED.seen_at)
`

func (r *GuestRepository) MarkSeen(ctx context.Context, ownerID uuid.UUID, at time.Time) error {
	_, err := r.conn(ctx).Exec(ctx, markGuestsSeenQuery, ownerID, at)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}
