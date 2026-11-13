package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// AppRepository keeps who added which HTML widget to their "Приложения".
type AppRepository struct {
	pool *pgxpool.Pool
}

func NewAppRepository(pool *pgxpool.Pool) *AppRepository {
	return &AppRepository{
		pool: pool,
	}
}

func (r *AppRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const installAppQuery = `
	INSERT INTO app_installs (
		app_id,
		user_id,
		created_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (app_id, user_id) DO NOTHING
`

func (r *AppRepository) Install(ctx context.Context, userID, app uuid.UUID, at time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, installAppQuery, app, userID, at.UTC()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const uninstallAppQuery = `
	DELETE FROM app_installs
	WHERE app_id = $1
		AND user_id = $2
`

func (r *AppRepository) Uninstall(ctx context.Context, userID, app uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, uninstallAppQuery, app, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// The apps of a section, newest first: $1 is the user, $2 the cursor, $3
// the limit.
const (
	listAllAppsQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE ext = 'html'
		AND id < $2
		AND EXISTS (SELECT 1 FROM app_installs WHERE app_id = documents.id)
		AND $1::uuid IS NOT NULL
	ORDER BY id DESC
	LIMIT $3
`
	listInstalledAppsQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE ext = 'html'
		AND id < $2
		AND id IN (SELECT app_id FROM app_installs WHERE user_id = $1)
	ORDER BY id DESC
	LIMIT $3
`
	listOwnAppsQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE ext = 'html'
		AND owner_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`
)

func (r *AppRepository) List(ctx context.Context, section post.AppSection, userID, before uuid.UUID, limit int) ([]*post.Document, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	query := listAllAppsQuery
	switch section {
	case post.AppsInstalled:
		query = listInstalledAppsQuery
	case post.AppsOwn:
		query = listOwnAppsQuery
	}

	rows, err := r.conn(ctx).Query(ctx, query, userID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectDocuments(rows)
}

const appUsersQuery = `
	SELECT
		app_id,
		count(*),
		bool_or(user_id = $2)
	FROM app_installs
	WHERE app_id = ANY($1)
	GROUP BY app_id
`

func (r *AppRepository) Users(ctx context.Context, viewerID uuid.UUID, apps []uuid.UUID) (map[uuid.UUID]service.AppUsers, error) {
	rows, err := r.conn(ctx).Query(ctx, appUsersQuery, apps, viewerID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	users := make(map[uuid.UUID]service.AppUsers, len(apps))
	for rows.Next() {
		var (
			app uuid.UUID
			u   service.AppUsers
		)
		if err = rows.Scan(&app, &u.Count, &u.Installed); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		users[app] = u
	}

	return users, errs.Wrap("rows.Err", rows.Err())
}

const deleteUserAppsQuery = `
	DELETE FROM app_installs
	WHERE user_id = $1
`

func (r *AppRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserAppsQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
