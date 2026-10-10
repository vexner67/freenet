package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
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

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *CommentRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// saveCommentQuery adds the comment and counts it in one statement.
const saveCommentQuery = `
	WITH saved AS (
		INSERT INTO comments (
			id,
			post_id,
			author_id,
			text,
			photo_ids,
			reply_to,
			created_at,
			from_wall
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING post_id
	)
	UPDATE posts
	SET comment_count = comment_count + 1
	WHERE id = (SELECT post_id FROM saved)
`

func (r *CommentRepository) Save(ctx context.Context, c *post.Comment) error {
	_, err := r.conn(ctx).Exec(ctx, saveCommentQuery, c.ID(), c.PostID(), c.AuthorID(), c.Text().String(), photoIDs(c.Photos()), nullable(c.ReplyTo()), c.CreatedAt(), c.FromWall())
	if isForeignKeyViolation(err) {
		// The comment answered, or else the post, is gone.
		if constraintOf(err) == "comments_reply_to_fkey" {
			return post.ErrCommentNotFound
		}
		return post.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getCommentQuery = `
	SELECT
		id,
		post_id,
		author_id,
		text,
		photo_ids,
		reply_to,
		like_count,
		created_at,
		from_wall
	FROM comments
	WHERE id = $1
`

func (r *CommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.Comment, error) {
	c, err := scanComment(r.conn(ctx).QueryRow(ctx, getCommentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrCommentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanComment", err)
	}

	return c, nil
}

const getCommentsQuery = `
	SELECT
		id,
		post_id,
		author_id,
		text,
		photo_ids,
		reply_to,
		like_count,
		created_at,
		from_wall
	FROM comments
	WHERE id = ANY($1)
`

func (r *CommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error) {
	rows, err := r.conn(ctx).Query(ctx, getCommentsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectComments(rows)
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
	tag, err := r.conn(ctx).Exec(ctx, deleteCommentQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
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
		photo_ids,
		reply_to,
		like_count,
		created_at,
		from_wall
	FROM comments
	WHERE post_id = $1
		AND id > $2
	ORDER BY id
	LIMIT $3
`

func (r *CommentRepository) ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error) {
	rows, err := r.conn(ctx).Query(ctx, listCommentsByPostQuery, postID, after, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectComments(rows)
}

// lastCommentsQuery reads the newest $2 comments of each post of $1 by
// the index on (post_id, id), one short scan a post.
const lastCommentsQuery = `
	SELECT
		c.id,
		c.post_id,
		c.author_id,
		c.text,
		c.photo_ids,
		c.reply_to,
		c.like_count,
		c.created_at,
		c.from_wall
	FROM posts p
	CROSS JOIN LATERAL (
		SELECT
			id,
			post_id,
			author_id,
			text,
			photo_ids,
			reply_to,
			like_count,
			created_at,
			from_wall
		FROM comments
		WHERE post_id = p.id
		ORDER BY id DESC
		LIMIT $2
	) c
	WHERE p.id = ANY($1)
		AND NOT p.archived
`

func (r *CommentRepository) LastByPosts(ctx context.Context, postIDs []uuid.UUID, limit int) ([]*post.Comment, error) {
	rows, err := r.conn(ctx).Query(ctx, lastCommentsQuery, postIDs, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectComments(rows)
}

// collectComments reads the comments of rows and closes them.
func collectComments(rows pgx.Rows) ([]*post.Comment, error) {
	defer rows.Close()

	var comments []*post.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, errs.Wrap("scanComment", err)
		}
		comments = append(comments, c)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return comments, nil
}

func scanComment(row pgx.Row) (*post.Comment, error) {
	var (
		s       post.StoredComment
		text    string
		photos  []uuid.UUID
		replyTo *uuid.UUID
	)

	if err := row.Scan(&s.ID, &s.PostID, &s.AuthorID, &text, &photos, &replyTo, &s.Likes, &s.CreatedAt, &s.FromWall); err != nil {
		return nil, err
	}

	if replyTo != nil {
		s.ReplyTo = *replyTo
	}

	if len(photos) == 0 {
		photos = nil // no photos, as a new comment has them
	}

	// Data comes from our own table and was validated on write.
	s.Content = post.RestoreCommentContent(post.CommentText(text), post.Photos(photos))
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreComment(s), nil
}

const lockPostQuery = `
	SELECT 1
	FROM posts
	WHERE id = $1
	FOR UPDATE
`

const deleteCommentsByPostQuery = `
	DELETE FROM comments
	WHERE post_id = $1
	RETURNING
		id,
		post_id,
		author_id,
		text,
		photo_ids,
		reply_to,
		like_count,
		created_at,
		from_wall
`

// DeleteByPost deletes the comments of a post about to be deleted,
// returning them, so that their photos go too; the post's counter is left
// alone. It locks the post first, in the transaction of ctx: a comment
// written meanwhile waits for the post's deletion and fails, instead of
// slipping in after this and being deleted unseen with the post.
func (r *CommentRepository) DeleteByPost(ctx context.Context, postID uuid.UUID) ([]*post.Comment, error) {
	if _, err := r.conn(ctx).Exec(ctx, lockPostQuery, postID); err != nil {
		return nil, errs.Wrap("r.conn.Exec", err)
	}

	rows, err := r.conn(ctx).Query(ctx, deleteCommentsByPostQuery, postID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectComments(rows)
}

const deleteCommentsOnWallQuery = `
	DELETE FROM comments
	WHERE post_id IN (
		SELECT id
		FROM posts
		WHERE author_id = $1
			OR wall_id = $1
	)
	RETURNING
		id,
		post_id,
		author_id,
		text,
		photo_ids,
		reply_to,
		like_count,
		created_at,
		from_wall
`

// DeleteOnPostsOf deletes the comments on the posts a user wrote and on
// those on their wall, returning them, before the posts go too.
func (r *CommentRepository) DeleteOnPostsOf(ctx context.Context, userID uuid.UUID) ([]*post.Comment, error) {
	rows, err := r.conn(ctx).Query(ctx, deleteCommentsOnWallQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectComments(rows)
}

// deleteCommentsByUserQuery deletes a user's comments and uncounts them in
// one statement.
const deleteCommentsByUserQuery = `
	WITH deleted AS (
		DELETE FROM comments
		WHERE author_id = $1
		RETURNING post_id
	)
	UPDATE posts p
	SET comment_count = p.comment_count - d.n
	FROM (
		SELECT post_id, count(*) AS n
		FROM deleted
		GROUP BY post_id
	) d
	WHERE p.id = d.post_id
`

func (r *CommentRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteCommentsByUserQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// nullable is id for a column where NULL means none.
func nullable(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}
