package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

type LikeRepository struct {
	pool *pgxpool.Pool
}

func NewLikeRepository(pool *pgxpool.Pool) *LikeRepository {
	return &LikeRepository{
		pool: pool,
	}
}

func (r *LikeRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addLikeQuery = `
	INSERT INTO remote_post_likes (
		id,
		post_id,
		user_id,
		created_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (post_id, user_id) DO NOTHING
`

func (r *LikeRepository) Add(ctx context.Context, postID uuid.UUID, l service.Liker, now time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, addLikeQuery, l.ID, postID, l.UserID, now.UTC()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeLikeQuery = `
	DELETE FROM remote_post_likes
	WHERE post_id = $1 AND user_id = $2
`

func (r *LikeRepository) Remove(ctx context.Context, postID, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, removeLikeQuery, postID, userID); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const countLikesQuery = `
	SELECT
		post_id,
		count(*),
		bool_or(user_id = $2)
	FROM remote_post_likes
	WHERE post_id = ANY($1)
	GROUP BY post_id
`

func (r *LikeRepository) Of(ctx context.Context, postIDs []uuid.UUID, reader uuid.UUID) (map[uuid.UUID]service.Likes, error) {
	rows, err := r.conn(ctx).Query(ctx, countLikesQuery, postIDs, reader)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}
	defer rows.Close()

	out := map[uuid.UUID]service.Likes{}
	for rows.Next() {
		var (
			id    uuid.UUID
			likes service.Likes
		)
		if err = rows.Scan(&id, &likes.Count, &likes.Liked); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		out[id] = likes
	}

	return out, errs.Wrap("rows.Err", rows.Err())
}

const listLikersQuery = `
	SELECT
		id,
		user_id
	FROM remote_post_likes
	WHERE post_id = $1
		AND ($2::uuid IS NULL OR id < $2)
	ORDER BY id DESC
	LIMIT $3
`

func (r *LikeRepository) Likers(ctx context.Context, postID, before uuid.UUID, limit int) ([]service.Liker, error) {
	var cursor *uuid.UUID
	if before != uuid.Nil {
		cursor = &before
	}

	rows, err := r.conn(ctx).Query(ctx, listLikersQuery, postID, cursor, limit)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	likers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[service.Liker])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return likers, nil
}
