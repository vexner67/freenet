package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// BookRepository reads the memberships by ID, for the post service to
// make its copy of them match. It works in the transaction of the
// context.
type BookRepository struct {
	pool *pgxpool.Pool
}

func NewBookRepository(pool *pgxpool.Pool) *BookRepository {
	return &BookRepository{
		pool: pool,
	}
}

const clubIDsQuery = `
	SELECT id
	FROM clubs
	WHERE id > $1
	ORDER BY id
	LIMIT $2
`

func (r *BookRepository) ClubIDs(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, clubIDsQuery, after, limit)
}

const memberIDsQuery = `
	SELECT user_id
	FROM club_members
	WHERE club_id = $1
		AND user_id > $2
	ORDER BY user_id
	LIMIT $3
`

func (r *BookRepository) MemberIDs(ctx context.Context, clubID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return r.ids(ctx, memberIDsQuery, clubID, after, limit)
}

// ids reads the rows of one ID.
func (r *BookRepository) ids(ctx context.Context, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := pgpool.Conn(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("pgpool.Conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// lockClubQuery takes an advisory lock that the transaction holds to its
// end; outside a transaction it would be gone at once.
const lockClubQuery = `SELECT pg_advisory_xact_lock(hashtextextended('feed_sources:' || $1::text, 0))`

func (r *BookRepository) LockClub(ctx context.Context, clubID uuid.UUID) error {
	if _, err := pgpool.Conn(ctx, r.pool).Exec(ctx, lockClubQuery, clubID); err != nil {
		return errs.Wrap("pgpool.Conn.Exec", err)
	}

	return nil
}
