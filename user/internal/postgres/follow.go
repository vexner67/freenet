package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

const foreignKeyViolationCode = "23503"

type FollowRepository struct {
	pool *pgxpool.Pool
}

func NewFollowRepository(pool *pgxpool.Pool) *FollowRepository {
	return &FollowRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *FollowRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// Following twice is not an error: the unique pair keeps one follow.
const followQuery = `
	INSERT INTO follows (
		id,
		follower_id,
		followee_id
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (follower_id, followee_id) DO NOTHING
`

func (r *FollowRepository) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, followQuery, id, followerID, followeeID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
			// The user was deleted between the check and the insert.
			return user.ErrNotFound
		}

		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const unfollowQuery = `
	DELETE FROM follows
	WHERE follower_id = $1
		AND followee_id = $2
`

func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, unfollowQuery, followerID, followeeID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const isFollowingQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM follows
		WHERE follower_id = $1
			AND followee_id = $2
	)
`

func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	var ok bool

	if err := r.conn(ctx).QueryRow(ctx, isFollowingQuery, followerID, followeeID).Scan(&ok); err != nil {
		return false, errs.Wrap("r.conn.QueryRow", err)
	}

	return ok, nil
}

const followCountsQuery = `
	SELECT
		(SELECT count(*) FROM follows WHERE followee_id = $1),
		(SELECT count(*) FROM follows WHERE follower_id = $1)
`

func (r *FollowRepository) Counts(ctx context.Context, userID uuid.UUID) (followers, following int, err error) {
	if err = r.conn(ctx).QueryRow(ctx, followCountsQuery, userID).Scan(&followers, &following); err != nil {
		return 0, 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return followers, following, nil
}

// The two lists differ only in which side of the follow is the user.
const (
	listFollowersQuery = `
	SELECT
		f.id,
		u.id,
		u.email,
		u.name,
		u.surname,
		u.nickname,
		u.status,
		u.gender,
		u.relationship,
		u.worldview,
		u.political_views,
		u.birthday,
		u.show_birthday,
		u.show_relationship,
		u.show_worldview,
		u.show_political_views,
		u.wall_write,
		u.messages_write,
		u.avatar_id,
		u.deactivated_at,
		u.purge_at,
		u.deactivation_reason
	FROM follows f
	JOIN users u ON u.id = f.follower_id
	WHERE f.followee_id = $1
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`

	listFollowingQuery = `
	SELECT
		f.id,
		u.id,
		u.email,
		u.name,
		u.surname,
		u.nickname,
		u.status,
		u.gender,
		u.relationship,
		u.worldview,
		u.political_views,
		u.birthday,
		u.show_birthday,
		u.show_relationship,
		u.show_worldview,
		u.show_political_views,
		u.wall_write,
		u.messages_write,
		u.avatar_id,
		u.deactivated_at,
		u.purge_at,
		u.deactivation_reason
	FROM follows f
	JOIN users u ON u.id = f.followee_id
	WHERE f.follower_id = $1
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`
)

func (r *FollowRepository) ListFollowers(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Followed, error) {
	return r.list(ctx, listFollowersQuery, userID, before, limit)
}

func (r *FollowRepository) ListFollowing(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Followed, error) {
	return r.list(ctx, listFollowingQuery, userID, before, limit)
}

func (r *FollowRepository) list(ctx context.Context, query string, userID, before uuid.UUID, limit int) ([]service.Followed, error) {
	if before == uuid.Nil {
		// Above every UUID: start from the newest follow.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, query, userID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []service.Followed
	for rows.Next() {
		var followID uuid.UUID

		u, err := scanUser(prefixedRow{row: rows, prefix: []any{&followID}})
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}

		found = append(found, service.Followed{FollowID: followID, User: u})
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const followingIDsQuery = `
	SELECT followee_id
	FROM follows
	WHERE follower_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *FollowRepository) FollowingIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, followingIDsQuery, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// prefixedRow scans leading columns into prefix and the rest into the
// destinations of the wrapped scan, so scanUser can read joined rows.
type prefixedRow struct {
	row    pgx.Row
	prefix []any
}

func (p prefixedRow) Scan(dest ...any) error {
	return p.row.Scan(append(p.prefix, dest...)...)
}
