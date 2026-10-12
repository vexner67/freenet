package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// RequestRepository keeps the requests to join closed clubs.
type RequestRepository struct {
	pool *pgxpool.Pool
}

func NewRequestRepository(pool *pgxpool.Pool) *RequestRepository {
	return &RequestRepository{
		pool: pool,
	}
}

func (r *RequestRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addRequestQuery = `
	INSERT INTO club_requests (
		id,
		club_id,
		user_id
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (club_id, user_id) DO NOTHING
`

func (r *RequestRepository) Add(ctx context.Context, clubID, userID uuid.UUID) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	_, err = r.conn(ctx).Exec(ctx, addRequestQuery, id, clubID, userID)
	if constraintOf(err) == "club_requests_club_id_fkey" {
		return club.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const deleteRequestQuery = `
	DELETE FROM club_requests
	WHERE club_id = $1
		AND user_id = $2
`

func (r *RequestRepository) Delete(ctx context.Context, clubID, userID uuid.UUID) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, deleteRequestQuery, clubID, userID)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() > 0, nil
}

const hasRequestQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM club_requests
		WHERE club_id = $1
			AND user_id = $2
	)
`

func (r *RequestRepository) Has(ctx context.Context, clubID, userID uuid.UUID) (bool, error) {
	var has bool

	if err := r.conn(ctx).QueryRow(ctx, hasRequestQuery, clubID, userID).Scan(&has); err != nil {
		return false, errs.Wrap("r.conn.QueryRow", err)
	}

	return has, nil
}

const listRequestsQuery = `
	SELECT
		id,
		user_id
	FROM club_requests
	WHERE club_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *RequestRepository) List(ctx context.Context, clubID, before uuid.UUID, limit int) ([]service.Request, error) {
	rows, err := r.conn(ctx).Query(ctx, listRequestsQuery, clubID, cursor(before), limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []service.Request
	for rows.Next() {
		var req service.Request

		if err = rows.Scan(&req.ID, &req.UserID); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		found = append(found, req)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const countRequestsQuery = `
	SELECT count(*)
	FROM club_requests
	WHERE club_id = $1
`

func (r *RequestRepository) Count(ctx context.Context, clubID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countRequestsQuery, clubID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const deleteClubRequestsQuery = `
	DELETE FROM club_requests
	WHERE club_id = $1
`

func (r *RequestRepository) DeleteByClub(ctx context.Context, clubID uuid.UUID) error {
	_, err := r.conn(ctx).Exec(ctx, deleteClubRequestsQuery, clubID)

	return errs.Wrap("r.conn.Exec", err)
}

const deleteUserRequestsQuery = `
	DELETE FROM club_requests
	WHERE user_id = $1
`

func (r *RequestRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.conn(ctx).Exec(ctx, deleteUserRequestsQuery, userID)

	return errs.Wrap("r.conn.Exec", err)
}
