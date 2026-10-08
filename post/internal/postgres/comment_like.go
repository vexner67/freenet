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

// CommentLikeRepository keeps the likes of comments as LikeRepository
// keeps those of posts: counted in the comment by the same statement.
type CommentLikeRepository struct {
	pool *pgxpool.Pool
}

func NewCommentLikeRepository(pool *pgxpool.Pool) *CommentLikeRepository {
	return &CommentLikeRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *CommentLikeRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// likeCommentQuery adds the like and counts it in one statement; a
// repeated like inserts nothing and adds 0.
const likeCommentQuery = `
	WITH liked AS (
		INSERT INTO comment_likes (
			comment_id,
			user_id,
			created_at
		)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING
		RETURNING 1
	)
	UPDATE comments
	SET like_count = like_count + (SELECT count(*) FROM liked)
	WHERE id = $1
	RETURNING like_count
`

func (r *CommentLikeRepository) Like(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	var likes int

	err := r.conn(ctx).QueryRow(ctx, likeCommentQuery, commentID, userID).Scan(&likes)
	if isForeignKeyViolation(err) {
		return 0, post.ErrCommentNotFound
	}
	if err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return likes, nil
}

const unlikeCommentQuery = `
	WITH unliked AS (
		DELETE FROM comment_likes
		WHERE comment_id = $1
			AND user_id = $2
		RETURNING 1
	)
	UPDATE comments
	SET like_count = like_count - (SELECT count(*) FROM unliked)
	WHERE id = $1
	RETURNING like_count
`

func (r *CommentLikeRepository) Unlike(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	var likes int

	err := r.conn(ctx).QueryRow(ctx, unlikeCommentQuery, commentID, userID).Scan(&likes)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, post.ErrCommentNotFound
	}
	if err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return likes, nil
}

const listLikedCommentsQuery = `
	SELECT comment_id
	FROM comment_likes
	WHERE user_id = $1
		AND comment_id = ANY($2)
`

func (r *CommentLikeRepository) Liked(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.conn(ctx).Query(ctx, listLikedCommentsQuery, userID, commentIDs)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	liked := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		liked[id] = true
	}

	return liked, nil
}

// deleteCommentLikesByUserQuery deletes a user's likes of comments and
// uncounts them in one statement.
const deleteCommentLikesByUserQuery = `
	WITH deleted AS (
		DELETE FROM comment_likes
		WHERE user_id = $1
		RETURNING comment_id
	)
	UPDATE comments c
	SET like_count = c.like_count - d.n
	FROM (
		SELECT comment_id, count(*) AS n
		FROM deleted
		GROUP BY comment_id
	) d
	WHERE c.id = d.comment_id
`

func (r *CommentLikeRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteCommentLikesByUserQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
