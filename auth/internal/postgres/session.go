package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/auth/internal/errs"
	"github.com/vexner67/freenet/auth/internal/session"
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
	if _, err := r.pool.Exec(ctx, deleteQuery, id); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
