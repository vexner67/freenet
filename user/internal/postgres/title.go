package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// TitleRepository keeps the titles of pages in titles.
type TitleRepository struct {
	pool *pgxpool.Pool
}

func NewTitleRepository(pool *pgxpool.Pool) *TitleRepository {
	return &TitleRepository{
		pool: pool,
	}
}

func (r *TitleRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addTitleQuery = `
	INSERT INTO titles (
		id,
		user_id,
		name,
		icon,
		granted_at
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *TitleRepository) AddTitle(ctx context.Context, t *user.Title) error {
	_, err := r.conn(ctx).Exec(ctx, addTitleQuery, t.ID(), t.UserID(), t.Name().String(), t.Icon().String(), t.GrantedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const listTitlesQuery = `
	SELECT
		id,
		name,
		icon,
		granted_at
	FROM titles
	WHERE user_id = $1
	ORDER BY id DESC
`

func (r *TitleRepository) Titles(ctx context.Context, userID uuid.UUID) ([]*user.Title, error) {
	rows, err := r.conn(ctx).Query(ctx, listTitlesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var titles []*user.Title
	for rows.Next() {
		var (
			id        uuid.UUID
			name      string
			icon      string
			grantedAt time.Time
		)
		if err = rows.Scan(&id, &name, &icon, &grantedAt); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		titles = append(titles, user.RestoreTitle(id, userID, user.TitleName(name), user.TitleIcon(icon), grantedAt))
	}

	return titles, errs.Wrap("rows.Err", rows.Err())
}

const deleteTitleQuery = `
	DELETE FROM titles
	WHERE id = $1
`

func (r *TitleRepository) DeleteTitle(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteTitleQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrTitleNotFound
	}

	return nil
}
