package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// foreignKeyViolation is the code of PostgreSQL for a row naming one
// that is not there.
const foreignKeyViolation = "23503"

type CommentRepository struct {
	pool *pgxpool.Pool
}

func NewCommentRepository(pool *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{
		pool: pool,
	}
}

func (r *CommentRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addCommentQuery = `
	INSERT INTO remote_post_comments (
		id,
		post_id,
		author_id,
		text,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *CommentRepository) AddComment(ctx context.Context, c *fedi.RemoteComment) error {
	_, err := r.conn(ctx).Exec(ctx, addCommentQuery, c.ID(), c.PostID(), c.AuthorID(), c.Text(), c.CreatedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return fedi.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const getCommentQuery = `
	SELECT
		id,
		post_id,
		author_id,
		text,
		created_at
	FROM remote_post_comments
	WHERE id = $1
`

func (r *CommentRepository) Comment(ctx context.Context, id uuid.UUID) (*fedi.RemoteComment, error) {
	rows, err := r.conn(ctx).Query(ctx, getCommentQuery, id)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	c, err := pgx.CollectOneRow(rows, scanComment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("pgx.CollectOneRow", err)
	}

	return c, nil
}

const deleteCommentQuery = `
	DELETE FROM remote_post_comments
	WHERE id = $1
`

func (r *CommentRepository) DeleteComment(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteCommentQuery, id); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const listCommentsQuery = `
	SELECT
		id,
		post_id,
		author_id,
		text,
		created_at
	FROM remote_post_comments
	WHERE post_id = $1
		AND id > $2
	ORDER BY id
	LIMIT $3
`

func (r *CommentRepository) Comments(ctx context.Context, postID, after uuid.UUID, limit int) ([]*fedi.RemoteComment, error) {
	rows, err := r.conn(ctx).Query(ctx, listCommentsQuery, postID, after, limit)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	comments, err := pgx.CollectRows(rows, scanComment)
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

const countCommentsQuery = `
	SELECT
		post_id,
		count(*)
	FROM remote_post_comments
	WHERE post_id = ANY($1)
	GROUP BY post_id
`

func (r *CommentRepository) CommentCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := r.conn(ctx).Query(ctx, countCommentsQuery, postIDs)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}
	defer rows.Close()

	out := map[uuid.UUID]int{}
	for rows.Next() {
		var (
			id    uuid.UUID
			count int
		)
		if err = rows.Scan(&id, &count); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		out[id] = count
	}

	return out, errs.Wrap("rows.Err", rows.Err())
}

func scanComment(row pgx.CollectableRow) (*fedi.RemoteComment, error) {
	var (
		id, postID, authorID uuid.UUID
		text                 string
		createdAt            time.Time
	)
	if err := row.Scan(&id, &postID, &authorID, &text, &createdAt); err != nil {
		return nil, err
	}

	return fedi.RestoreRemoteComment(id, postID, authorID, text, createdAt), nil
}
