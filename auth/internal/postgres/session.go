package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/errs"
)

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{
		pool: pool,
	}
}

const saveQuery = `
	INSERT INTO sessions (
		id,
		user_id,
		refresh_token_hash,
		created_at,
		last_used_at,
		ip,
		user_agent
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
`

func (r *SessionRepository) Save(ctx context.Context, s *session.Session) error {
	if _, err := r.pool.Exec(
		ctx,
		saveQuery,
		s.ID(),
		s.UserID(),
		s.RefreshTokenHash(),
		s.CreatedAt(),
		s.UsedAt(),
		s.Device().IP(),
		s.Device().Agent(),
	); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const deleteQuery = `
	DELETE FROM sessions
	WHERE id = $1
`

func (r *SessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deleteQuery, id)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return service.ErrSessionNotFound
	}

	return nil
}

const getQuery = `
	SELECT
		id,
		user_id,
		refresh_token_hash,
		created_at,
		last_used_at,
		ip,
		user_agent
	FROM sessions
	WHERE id = $1
`

func (r *SessionRepository) Get(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	s, err := scanSession(r.pool.QueryRow(ctx, getQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, service.ErrSessionNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanSession", err)
	}

	return s, nil
}

func scanSession(row pgx.Row) (*session.Session, error) {
	var (
		id, userID        uuid.UUID
		refreshTokenHash  string
		createdAt, usedAt time.Time
		ip, agent         string
	)

	if err := row.Scan(
		&id,
		&userID,
		&refreshTokenHash,
		&createdAt,
		&usedAt,
		&ip,
		&agent,
	); err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	return session.Restore(id, userID, refreshTokenHash, createdAt, usedAt, session.NewDevice(ip, agent)), nil
}

// The hash in WHERE makes the rotation a compare-and-swap.
const rotateQuery = `
	UPDATE sessions
	SET refresh_token_hash = $3,
		last_used_at = $4,
		ip = $5,
		user_agent = $6
	WHERE id = $1
		AND refresh_token_hash = $2
`

func (r *SessionRepository) Rotate(ctx context.Context, rotated *session.Session, oldHash string) error {
	tag, err := r.pool.Exec(
		ctx,
		rotateQuery,
		rotated.ID(),
		oldHash,
		rotated.RefreshTokenHash(),
		rotated.UsedAt(),
		rotated.Device().IP(),
		rotated.Device().Agent(),
	)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return service.ErrSessionNotFound
	}

	return nil
}

const listByUserQuery = `
	SELECT
		id,
		user_id,
		refresh_token_hash,
		created_at,
		last_used_at,
		ip,
		user_agent
	FROM sessions
	WHERE user_id = $1
		AND last_used_at > $2
	ORDER BY last_used_at DESC, id DESC
	LIMIT $3
`

func (r *SessionRepository) ListByUser(ctx context.Context, userID uuid.UUID, usedAfter time.Time, limit int) ([]*session.Session, error) {
	rows, err := r.pool.Query(ctx, listByUserQuery, userID, usedAfter, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var sessions []*session.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, errs.Wrap("scanSession", err)
		}

		sessions = append(sessions, s)
	}
	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return sessions, nil
}

const deleteOwnedQuery = `
	DELETE FROM sessions
	WHERE id = $2
		AND user_id = $1
`

func (r *SessionRepository) DeleteOwned(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deleteOwnedQuery, userID, id)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return service.ErrSessionNotFound
	}

	return nil
}

const deleteOthersQuery = `
	DELETE FROM sessions
	WHERE user_id = $1
		AND id <> $2
`

func (r *SessionRepository) DeleteOthers(ctx context.Context, userID, keep uuid.UUID) (int, error) {
	tag, err := r.pool.Exec(ctx, deleteOthersQuery, userID, keep)
	if err != nil {
		return 0, errs.Wrap("r.pool.Exec", err)
	}

	return int(tag.RowsAffected()), nil
}
