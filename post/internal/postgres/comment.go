package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type CommentRepository struct {
	pool *pgxpool.Pool
}

func NewCommentRepository(pool *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{
		pool: pool,
	}
}

// saveCommentQuery adds the comment and counts it in one statement.
const saveCommentQuery = `
	WITH saved AS (
		INSERT INTO comments (
			id,
			post_id,
			author_id,
			text,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING post_id
	)
	UPDATE posts
	SET comment_count = comment_count + 1
	WHERE id = (SELECT post_id FROM saved)
`

func (r *CommentRepository) Save(ctx context.Context, c *post.Comment) error {
	_, err := r.pool.Exec(ctx, saveCommentQuery, c.ID(), c.PostID(), c.AuthorID(), c.Text().String(), c.CreatedAt())
	if isForeignKeyViolation(err) {
		return post.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
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
	FROM comments
	WHERE id = $1
`

func (r *CommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.Comment, error) {
	c, err := scanComment(r.pool.QueryRow(ctx, getCommentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrCommentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanComment", err)
	}

	return c, nil
}

// deleteCommentQuery removes the comment and uncounts it in one statement;
// no row is updated when there was no such comment.
const deleteCommentQuery = `
	WITH deleted AS (
		DELETE FROM comments
		WHERE id = $1
		RETURNING post_id
	)
	UPDATE posts
	SET comment_count = comment_count - 1
	WHERE id = (SELECT post_id FROM deleted)
`

func (r *CommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deleteCommentQuery, id)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrCommentNotFound
	}

	return nil
}

// uuid.Nil is below every UUID, so "id > $2" with it starts from the first.
const listCommentsByPostQuery = `
	SELECT
		id,
		post_id,
		author_id,
		text,
		created_at
	FROM comments
	WHERE post_id = $1
		AND id > $2
	ORDER BY id
	LIMIT $3
`

func (r *CommentRepository) ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error) {
	rows, err := r.pool.Query(ctx, listCommentsByPostQuery, postID, after, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var comments []*post.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, errs.Wrap("scanComment", err)
		}
		comments = append(comments, c)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return comments, nil
}

func scanComment(row pgx.Row) (*post.Comment, error) {
	var (
		id, postID, authorID uuid.UUID
		text                 string
		createdAt            time.Time
	)

	if err := row.Scan(&id, &postID, &authorID, &text, &createdAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	return post.RestoreComment(id, postID, authorID, post.CommentText(text), createdAt.UTC()), nil
}
