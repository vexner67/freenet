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
		created_at
	)
	VALUES ($1, $2, $3, $4)
`

func (r *SessionRepository) Save(ctx context.Context, s *session.Session) error {
	if _, err := r.pool.Exec(
		ctx,
		saveQuery,
		s.ID(),
		s.UserID(),
		s.RefreshTokenHash(),
		s.CreatedAt(),
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
		created_at
	FROM sessions
	WHERE id = $1
`

func (r *SessionRepository) Get(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	var (
		sessionID, userID uuid.UUID
		refreshTokenHash  string
		createdAt         time.Time
	)

	err := r.pool.QueryRow(ctx, getQuery, id).Scan(
		&sessionID,
		&userID,
		&refreshTokenHash,
		&createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, service.ErrSessionNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return session.Restore(sessionID, userID, refreshTokenHash, createdAt), nil
}

// The hash in WHERE makes the rotation a compare-and-swap.
const rotateQuery = `
	UPDATE sessions
	SET refresh_token_hash = $3
	WHERE id = $1
		AND refresh_token_hash = $2
`

func (r *SessionRepository) Rotate(ctx context.Context, id uuid.UUID, oldHash, newHash string) error {
	tag, err := r.pool.Exec(ctx, rotateQuery, id, oldHash, newHash)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return service.ErrSessionNotFound
	}

	return nil
}
