package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

const foreignKeyViolationCode = "23503"

// isForeignKeyViolation: a like or a comment refers to a post that is gone.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode
}

type LikeRepository struct {
	pool *pgxpool.Pool
}

func NewLikeRepository(pool *pgxpool.Pool) *LikeRepository {
	return &LikeRepository{
		pool: pool,
	}
}

// likePostQuery adds the like and counts it in one statement, so the
// counter cannot drift. A repeated like inserts nothing and adds 0.
const likePostQuery = `
	WITH liked AS (
		INSERT INTO post_likes (
			post_id,
			user_id,
			created_at
		)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING
		RETURNING 1
	)
	UPDATE posts
	SET like_count = like_count + (SELECT count(*) FROM liked)
	WHERE id = $1
	RETURNING like_count
`

func (r *LikeRepository) Like(ctx context.Context, postID, userID uuid.UUID) (int, error) {
	var likes int

	err := r.pool.QueryRow(ctx, likePostQuery, postID, userID).Scan(&likes)
	if isForeignKeyViolation(err) {
		return 0, post.ErrNotFound
	}
	if err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return likes, nil
}

const unlikePostQuery = `
	WITH unliked AS (
		DELETE FROM post_likes
		WHERE post_id = $1
			AND user_id = $2
		RETURNING 1
	)
	UPDATE posts
	SET like_count = like_count - (SELECT count(*) FROM unliked)
	WHERE id = $1
	RETURNING like_count
`

func (r *LikeRepository) Unlike(ctx context.Context, postID, userID uuid.UUID) (int, error) {
	var likes int

	err := r.pool.QueryRow(ctx, unlikePostQuery, postID, userID).Scan(&likes)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, post.ErrNotFound
	}
	if err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return likes, nil
}

const listLikedPostsQuery = `
	SELECT post_id
	FROM post_likes
	WHERE user_id = $1
		AND post_id = ANY($2)
`

func (r *LikeRepository) Liked(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.pool.Query(ctx, listLikedPostsQuery, userID, postIDs)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
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
