package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// favesFrom is where a page of faves starts: after the cursor, or above
// every like for the first page.
func favesFrom(after *service.FaveCursor) (time.Time, uuid.UUID) {
	if after == nil {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), uuid.Max
	}

	return after.LikedAt, after.ID
}

// faves reads rows of what is liked and when, and closes them.
func faves(ctx context.Context, conn pgpool.Querier, query string, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	at, id := favesFrom(after)

	rows, err := conn.Query(ctx, query, userID, at, id, limit)
	if err != nil {
		return nil, errs.Wrap("conn.Query", err)
	}

	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.Fave, error) {
		var f service.Fave
		err := row.Scan(&f.ID, &f.LikedAt)
		f.LikedAt = f.LikedAt.UTC()
		return f, err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return found, nil
}

// listFavePostsQuery reads post_likes_faves_idx; likes of archived posts
// are skipped.
const listFavePostsQuery = `
	SELECT
		l.post_id,
		l.created_at
	FROM post_likes l
	JOIN posts p ON p.id = l.post_id
	WHERE l.user_id = $1
		AND (l.created_at, l.post_id) < ($2, $3)
		AND NOT p.archived
		AND NOT post_hidden(p.wall_id, p.author_id, p.from_wall)
	ORDER BY l.created_at DESC, l.post_id DESC
	LIMIT $4
`

func (r *LikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	return faves(ctx, r.conn(ctx), listFavePostsQuery, userID, after, limit)
}

// The likes of a person are counted as they come and go, by the triggers
// of the tables of likes: fave_counts has no row for who never liked.
// Unlike the lists, the numbers count what is in the archive too, but
// not what the pages deleted since wrote: that is taken off as the number
// is read, starting from the few deleted pages.
const countFavePostsQuery = `
	SELECT COALESCE((
		SELECT posts
		FROM fave_counts
		WHERE user_id = $1
	), 0) - (
		SELECT count(*)
		FROM post_likes l
		WHERE l.user_id = $1
			AND l.post_id IN (
				SELECT p.id
				FROM hidden_users h
				JOIN posts p ON p.author_id = h.user_id
				WHERE NOT p.from_wall
				UNION
				SELECT p.id
				FROM hidden_users h
				JOIN posts p ON p.wall_id = h.user_id
			)
	)
`

func (r *LikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countFavePostsQuery, userID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

// listFaveCommentsQuery reads comment_likes_faves_idx; likes of comments
// under archived posts are skipped.
const listFaveCommentsQuery = `
	SELECT
		l.comment_id,
		l.created_at
	FROM comment_likes l
	JOIN comments c ON c.id = l.comment_id
	JOIN posts p ON p.id = c.post_id
	WHERE l.user_id = $1
		AND (l.created_at, l.comment_id) < ($2, $3)
		AND NOT p.archived
		AND NOT post_hidden(p.wall_id, p.author_id, p.from_wall)
		AND NOT (user_hidden(c.author_id) AND NOT c.from_wall)
	ORDER BY l.created_at DESC, l.comment_id DESC
	LIMIT $4
`

func (r *CommentLikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	return faves(ctx, r.conn(ctx), listFaveCommentsQuery, userID, after, limit)
}

const countFaveCommentsQuery = `
	SELECT COALESCE((
		SELECT comments
		FROM fave_counts
		WHERE user_id = $1
	), 0) - (
		SELECT count(*)
		FROM comment_likes l
		WHERE l.user_id = $1
			AND l.comment_id IN (
				SELECT c.id
				FROM hidden_users h
				JOIN comments c ON c.author_id = h.user_id
				WHERE NOT c.from_wall
				UNION
				SELECT c.id
				FROM hidden_users h
				JOIN posts p ON p.author_id = h.user_id
				JOIN comments c ON c.post_id = p.id
				WHERE NOT p.from_wall
				UNION
				SELECT c.id
				FROM hidden_users h
				JOIN posts p ON p.wall_id = h.user_id
				JOIN comments c ON c.post_id = p.id
			)
	)
`

func (r *CommentLikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countFaveCommentsQuery, userID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

// listFavePhotosQuery reads album_photo_likes_faves_idx.
const listFavePhotosQuery = `
	SELECT
		l.photo_id,
		l.created_at
	FROM album_photo_likes l
	JOIN album_photos p ON p.id = l.photo_id
	JOIN albums a ON a.id = p.album_id
	WHERE l.user_id = $1
		AND (l.created_at, l.photo_id) < ($2, $3)
		AND NOT user_hidden(p.author_id)
		AND NOT user_hidden(a.owner_id)
	ORDER BY l.created_at DESC, l.photo_id DESC
	LIMIT $4
`

func (l *AlbumPhotoLikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	return faves(ctx, l.r.conn(ctx), listFavePhotosQuery, userID, after, limit)
}

const countFavePhotosQuery = `
	SELECT COALESCE((
		SELECT photos
		FROM fave_counts
		WHERE user_id = $1
	), 0) - (
		SELECT count(*)
		FROM album_photo_likes l
		WHERE l.user_id = $1
			AND l.photo_id IN (
				SELECT p.id
				FROM hidden_users h
				JOIN album_photos p ON p.author_id = h.user_id
				UNION
				SELECT p.id
				FROM hidden_users h
				JOIN albums a ON a.owner_id = h.user_id
				JOIN album_photos p ON p.album_id = a.id
			)
	)
`

func (l *AlbumPhotoLikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	if err := l.r.conn(ctx).QueryRow(ctx, countFavePhotosQuery, userID).Scan(&n); err != nil {
		return 0, errs.Wrap("l.r.conn.QueryRow", err)
	}

	return n, nil
}

const getAlbumPhotosQuery = `
	SELECT
		id,
		album_id,
		author_id,
		description,
		created_at,
		like_count,
		comment_count - hidden_photo_comments(id) AS comment_count,
		nsfw
	FROM album_photos
	WHERE id = ANY($1)
`

// GetMany returns the photos that exist, in no order.
func (p *AlbumPhotoRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.AlbumPhoto, error) {
	rows, err := p.r.conn(ctx).Query(ctx, getAlbumPhotosQuery, ids)
	if err != nil {
		return nil, errs.Wrap("p.r.conn.Query", err)
	}

	return collectAlbumPhotos(rows)
}

// listPostsByAuthorQuery reads posts_author_id_idx: what a person wrote,
// on whichever wall, but for what was archived on the walls of others.
const listPostsByAuthorQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count - hidden_comments(id) AS comment_count,
		repost_count - unseen_reposts(id) AS repost_count,
		edited_at,
		from_wall,
		signed,
		suggested,
		avatar_update,
		shared_photo_id,
		archived,
		comments_closed,
		nsfw,
		polled,
		document_ids,
		EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	FROM posts
	WHERE author_id = $1
		AND id < $2
		AND (NOT archived OR wall_id = $1)
		AND NOT suggested
		AND NOT user_hidden(wall_id)
	ORDER BY id DESC
	LIMIT $3
`

func (r *PostRepository) ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listPostsByAuthorQuery, authorID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectPosts(rows)
}

const countPostsByAuthorQuery = `
	SELECT count(*)
	FROM posts
	WHERE author_id = $1
		AND (NOT archived OR wall_id = $1)
		AND NOT suggested
		AND NOT user_hidden(wall_id)
`

func (r *PostRepository) CountByAuthor(ctx context.Context, authorID uuid.UUID) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countPostsByAuthorQuery, authorID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}
