package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
)

type StoryLikeRepository struct {
	pool *pgxpool.Pool
}

func NewStoryLikeRepository(pool *pgxpool.Pool) *StoryLikeRepository {
	return &StoryLikeRepository{
		pool: pool,
	}
}

func (r *StoryLikeRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const likeStoryQuery = `
	INSERT INTO story_likes (
		story_id,
		user_id,
		created_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (story_id, user_id) DO NOTHING
`

func (r *StoryLikeRepository) Like(ctx context.Context, storyID, userID uuid.UUID, at time.Time) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, likeStoryQuery, storyID, userID, at)
	if isForeignKeyViolation(err) {
		return false, post.ErrStoryNotFound
	}
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() == 1, nil
}

const unlikeStoryQuery = `
	DELETE FROM story_likes
	WHERE story_id = $1
		AND user_id = $2
`

func (r *StoryLikeRepository) Unlike(ctx context.Context, storyID, userID uuid.UUID) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, unlikeStoryQuery, storyID, userID)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() == 1, nil
}

const likedStoriesQuery = `
	SELECT story_id
	FROM story_likes
	WHERE user_id = $1
		AND story_id = ANY($2)
`

func (r *StoryLikeRepository) Liked(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.conn(ctx).Query(ctx, likedStoriesQuery, userID, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	liked := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		liked[id] = true
	}

	return liked, nil
}

const countStoryLikesQuery = `
	SELECT
		story_id,
		count(*)
	FROM story_likes
	WHERE story_id = ANY($1)
		AND NOT user_hidden(user_id)
	GROUP BY story_id
`

func (r *StoryLikeRepository) Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	return countByStory(ctx, r.conn(ctx), countStoryLikesQuery, ids)
}

type StoryCommentRepository struct {
	pool *pgxpool.Pool
}

func NewStoryCommentRepository(pool *pgxpool.Pool) *StoryCommentRepository {
	return &StoryCommentRepository{
		pool: pool,
	}
}

func (r *StoryCommentRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveStoryCommentQuery = `
	INSERT INTO story_comments (
		id,
		story_id,
		author_id,
		text,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *StoryCommentRepository) Save(ctx context.Context, c *post.StoryComment) error {
	_, err := r.conn(ctx).Exec(ctx, saveStoryCommentQuery, c.ID(), c.StoryID(), c.AuthorID(), c.Text().String(), c.CreatedAt())
	if isForeignKeyViolation(err) {
		return post.ErrStoryNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getStoryCommentQuery = `
	SELECT
		id,
		story_id,
		author_id,
		text,
		created_at
	FROM story_comments
	WHERE id = $1
`

func (r *StoryCommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.StoryComment, error) {
	c, err := scanStoryComment(r.conn(ctx).QueryRow(ctx, getStoryCommentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrStoryCommentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanStoryComment", err)
	}

	return c, nil
}

const deleteStoryCommentQuery = `
	DELETE FROM story_comments
	WHERE id = $1
`

func (r *StoryCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteStoryCommentQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrStoryCommentNotFound
	}

	return nil
}

// listStoryCommentsQuery pages after the ID $2, uuid.Nil from the first.
const listStoryCommentsQuery = `
	SELECT
		id,
		story_id,
		author_id,
		text,
		created_at
	FROM story_comments
	WHERE story_id = $1
		AND id > $2
		AND NOT user_hidden(author_id)
	ORDER BY id
	LIMIT $3
`

func (r *StoryCommentRepository) List(ctx context.Context, storyID, after uuid.UUID, limit int) ([]*post.StoryComment, error) {
	rows, err := r.conn(ctx).Query(ctx, listStoryCommentsQuery, storyID, after, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.StoryComment, error) {
		return scanStoryComment(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

const countStoryCommentsQuery = `
	SELECT
		story_id,
		count(*)
	FROM story_comments
	WHERE story_id = ANY($1)
		AND NOT user_hidden(author_id)
	GROUP BY story_id
`

func (r *StoryCommentRepository) Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	return countByStory(ctx, r.conn(ctx), countStoryCommentsQuery, ids)
}

func scanStoryComment(row pgx.Row) (*post.StoryComment, error) {
	var s post.StoredStoryComment
	var text string

	if err := row.Scan(&s.ID, &s.StoryID, &s.AuthorID, &text, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Text = post.CommentText(text)

	return post.RestoreStoryComment(s), nil
}

// countByStory runs a query of (story_id, count) rows.
func countByStory(ctx context.Context, conn pgpool.Querier, query string, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := conn.Query(ctx, query, ids)
	if err != nil {
		return nil, errs.Wrap("conn.Query", err)
	}
	defer rows.Close()

	counts := make(map[uuid.UUID]int)
	for rows.Next() {
		var (
			id    uuid.UUID
			count int
		)
		if err = rows.Scan(&id, &count); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		counts[id] = count
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return counts, nil
}
