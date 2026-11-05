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

const readersQuery = `
	SELECT id
	FROM users
	WHERE id > $1
	ORDER BY id
	LIMIT $2
`

func (r *FollowRepository) Readers(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, readersQuery, after, limit)
}

const followeesQuery = `
	SELECT followee_id
	FROM follows
	WHERE follower_id = $1
		AND followee_id > $2
	ORDER BY followee_id
	LIMIT $3
`

func (r *FollowRepository) Followees(ctx context.Context, readerID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, followeesQuery, readerID, after, limit)
}

// ids reads the rows of one ID.
func (r *FollowRepository) ids(ctx context.Context, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// lockReaderQuery takes an advisory lock that the transaction holds to
// its end; outside a transaction it would be gone at once.
const lockReaderQuery = `SELECT pg_advisory_xact_lock(hashtextextended('feed_sources:' || $1::text, 0))`

func (r *FollowRepository) LockReader(ctx context.Context, readerID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, lockReaderQuery, readerID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
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

// Friends follow each other: the user's follow of a friend (its ID is
// the cursor) meets the friend's follow back.
const mutualJoin = `
	JOIN follows b ON b.follower_id = f.followee_id AND b.followee_id = f.follower_id`

// The lists of friends read the user's follows by its index; a request is
// a follow of the user not followed back, an outgoing one the user's
// follow not followed back.
const (
	listFriendsQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f` + mutualJoin + `
	JOIN users u ON u.id = f.followee_id
	WHERE f.follower_id = $1
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`
	countFriendsQuery = `
	SELECT count(*)
	FROM follows f` + mutualJoin + `
	WHERE f.follower_id = $1
`
	listOnlineFriendsQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f` + mutualJoin + `
	JOIN users u ON u.id = f.followee_id
	JOIN presence p ON p.user_id = f.followee_id
	WHERE f.follower_id = $1
		AND p.last_seen_at > $2
		AND u.deactivated_at IS NULL
		AND f.id < $3
	ORDER BY f.id DESC
	LIMIT $4
`
	countOnlineFriendsQuery = `
	SELECT count(*)
	FROM follows f` + mutualJoin + `
	JOIN users u ON u.id = f.followee_id
	JOIN presence p ON p.user_id = f.followee_id
	WHERE f.follower_id = $1
		AND p.last_seen_at > $2
		AND u.deactivated_at IS NULL
`
	// The viewer's friendship with each is two lookups of the
	// (follower, followee) key.
	commonFriendCondition = `
		AND EXISTS (
			SELECT 1
			FROM follows v
			JOIN follows w ON w.follower_id = v.followee_id AND w.followee_id = v.follower_id
			WHERE v.follower_id = $2
				AND v.followee_id = f.followee_id
		)`
	listCommonFriendsQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f` + mutualJoin + `
	JOIN users u ON u.id = f.followee_id
	WHERE f.follower_id = $1` + commonFriendCondition + `
		AND f.id < $3
	ORDER BY f.id DESC
	LIMIT $4
`
	countCommonFriendsQuery = `
	SELECT count(*)
	FROM follows f` + mutualJoin + `
	WHERE f.follower_id = $1` + commonFriendCondition + `
`
	notFollowedBack = `
		AND NOT EXISTS (
			SELECT 1
			FROM follows b
			WHERE b.follower_id = $1
				AND b.followee_id = f.follower_id
		)`
	listRequestsQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f
	JOIN users u ON u.id = f.follower_id
	WHERE f.followee_id = $1
		AND NOT f.declined` + notFollowedBack + `
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`
	countRequestsQuery = `
	SELECT count(*)
	FROM follows f
	WHERE f.followee_id = $1
		AND NOT f.declined` + notFollowedBack + `
`
	listFollowersQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f
	JOIN users u ON u.id = f.follower_id
	WHERE f.followee_id = $1` + notFollowedBack + `
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`
	countFollowersQuery = `
	SELECT count(*)
	FROM follows f
	WHERE f.followee_id = $1` + notFollowedBack + `
`
	notFollowingBack = `
		AND NOT EXISTS (
			SELECT 1
			FROM follows b
			WHERE b.follower_id = f.followee_id
				AND b.followee_id = $1
		)`
	listOutgoingQuery = `
	SELECT
		f.id,` + qualifiedUserColumns + `
	FROM follows f
	JOIN users u ON u.id = f.followee_id
	WHERE f.follower_id = $1` + notFollowingBack + `
		AND f.id < $2
	ORDER BY f.id DESC
	LIMIT $3
`
	countOutgoingQuery = `
	SELECT count(*)
	FROM follows f
	WHERE f.follower_id = $1` + notFollowingBack + `
`
)

// friendQueries are the list and the count of each section.
var friendQueries = map[user.FriendSection][2]string{
	user.FriendsAll:      {listFriendsQuery, countFriendsQuery},
	user.FriendsOnline:   {listOnlineFriendsQuery, countOnlineFriendsQuery},
	user.FriendsCommon:   {listCommonFriendsQuery, countCommonFriendsQuery},
	user.FriendRequests:  {listRequestsQuery, countRequestsQuery},
	user.FriendFollowers: {listFollowersQuery, countFollowersQuery},
	user.FriendsOutgoing: {listOutgoingQuery, countOutgoingQuery},
}

// friendArgs are the parameters of a section's queries but the cursor and
// the limit, which come last.
func friendArgs(q service.FriendQuery) []any {
	switch q.Section {
	case user.FriendsOnline:
		return []any{q.UserID, q.Since}
	case user.FriendsCommon:
		return []any{q.UserID, q.ViewerID}
	default:
		return []any{q.UserID}
	}
}

func (r *FollowRepository) ListFriends(ctx context.Context, q service.FriendQuery, before uuid.UUID, limit int) ([]service.Followed, error) {
	queries, ok := friendQueries[q.Section]
	if !ok {
		return nil, user.ErrInvalidFriendSection
	}

	if before == uuid.Nil {
		// Above every UUID: start from the newest follow.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, queries[0], append(friendArgs(q), before, limit)...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	found, err := scanFollowed(rows)
	if err != nil {
		return nil, errs.Wrap("scanFollowed", err)
	}

	return found, nil
}

func (r *FollowRepository) CountFriends(ctx context.Context, q service.FriendQuery) (int, error) {
	queries, ok := friendQueries[q.Section]
	if !ok {
		return 0, user.ErrInvalidFriendSection
	}

	var n int
	if err := r.conn(ctx).QueryRow(ctx, queries[1], friendArgs(q)...).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const declineQuery = `
	UPDATE follows
	SET declined = true
	WHERE follower_id = $1
		AND followee_id = $2
`

func (r *FollowRepository) Decline(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, declineQuery, followerID, followeeID)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() > 0, nil
}

// scanFollowed reads rows of a follow ID and a user, and closes them.
func scanFollowed(rows pgx.Rows) ([]service.Followed, error) {
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

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
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
