package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// AlbumPhotoLikeRepository is the service.AlbumPhotoLikeRepository side
// of AlbumRepository: likes counted in the photo by the same statement.
type AlbumPhotoLikeRepository struct {
	r *AlbumRepository
}

// PhotoLikes is the same repository as service.AlbumPhotoLikeRepository.
func (r *AlbumRepository) PhotoLikes() *AlbumPhotoLikeRepository {
	return &AlbumPhotoLikeRepository{r: r}
}

const likeAlbumPhotoQuery = `
	WITH liked AS (
		INSERT INTO album_photo_likes (
			photo_id,
			user_id,
			created_at
		)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING
		RETURNING 1
	)
	UPDATE album_photos
	SET like_count = like_count + (SELECT count(*) FROM liked)
	WHERE id = $1
	RETURNING like_count
`

func (l *AlbumPhotoLikeRepository) Like(ctx context.Context, photoID, userID uuid.UUID) (int, error) {
	return l.change(ctx, likeAlbumPhotoQuery, photoID, userID)
}

const unlikeAlbumPhotoQuery = `
	WITH unliked AS (
		DELETE FROM album_photo_likes
		WHERE photo_id = $1
			AND user_id = $2
		RETURNING 1
	)
	UPDATE album_photos
	SET like_count = like_count - (SELECT count(*) FROM unliked)
	WHERE id = $1
	RETURNING like_count
`

func (l *AlbumPhotoLikeRepository) Unlike(ctx context.Context, photoID, userID uuid.UUID) (int, error) {
	return l.change(ctx, unlikeAlbumPhotoQuery, photoID, userID)
}

func (l *AlbumPhotoLikeRepository) change(ctx context.Context, query string, photoID, userID uuid.UUID) (int, error) {
	var likes int

	err := l.r.conn(ctx).QueryRow(ctx, query, photoID, userID).Scan(&likes)
	if isForeignKeyViolation(err) || errors.Is(err, pgx.ErrNoRows) {
		return 0, post.ErrAlbumPhotoNotFound
	}
	if err != nil {
		return 0, errs.Wrap("l.r.conn.QueryRow", err)
	}

	return likes, nil
}

const listLikedAlbumPhotosQuery = `
	SELECT photo_id
	FROM album_photo_likes
	WHERE user_id = $1
		AND photo_id = ANY($2)
`

func (l *AlbumPhotoLikeRepository) Liked(ctx context.Context, userID uuid.UUID, photoIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := l.r.conn(ctx).Query(ctx, listLikedAlbumPhotosQuery, userID, photoIDs)
	if err != nil {
		return nil, errs.Wrap("l.r.conn.Query", err)
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

const listAlbumPhotoLikersQuery = `
	SELECT
		user_id,
		created_at
	FROM album_photo_likes
	WHERE photo_id = $1
		AND (created_at, user_id) < ($2, $3)
	ORDER BY created_at DESC, user_id DESC
	LIMIT $4
`

func (l *AlbumPhotoLikeRepository) Likers(ctx context.Context, photoID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	at, userID := likersFrom(after)

	rows, err := l.r.conn(ctx).Query(ctx, listAlbumPhotoLikersQuery, photoID, at, userID, limit)
	if err != nil {
		return nil, errs.Wrap("l.r.conn.Query", err)
	}

	return collectLikers(rows)
}

// PhotoCommentRepository is the service.PhotoCommentRepository side of
// AlbumRepository: comments counted in the photo by the same statement.
type PhotoCommentRepository struct {
	r *AlbumRepository
}

// PhotoComments is the same repository as service.PhotoCommentRepository.
func (r *AlbumRepository) PhotoComments() *PhotoCommentRepository {
	return &PhotoCommentRepository{r: r}
}

const savePhotoCommentQuery = `
	WITH saved AS (
		INSERT INTO photo_comments (
			id,
			photo_id,
			author_id,
			text,
			reply_to,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING photo_id
	)
	UPDATE album_photos p
	SET comment_count = p.comment_count + 1
	FROM saved
	WHERE p.id = saved.photo_id
`

func (c *PhotoCommentRepository) Save(ctx context.Context, pc *post.PhotoComment) error {
	_, err := c.r.conn(ctx).Exec(ctx, savePhotoCommentQuery, pc.ID(), pc.PhotoID(), pc.AuthorID(), pc.Text().String(), nullable(pc.ReplyTo()), pc.CreatedAt())
	if isForeignKeyViolation(err) {
		// The comment to answer was deleted meanwhile, or the photo was.
		if constraintOf(err) == "photo_comments_reply_to_fkey" {
			return post.ErrCommentNotFound
		}

		return post.ErrAlbumPhotoNotFound
	}
	if err != nil {
		return errs.Wrap("c.r.conn.Exec", err)
	}

	return nil
}

const getPhotoCommentQuery = `
	SELECT
		id,
		photo_id,
		author_id,
		text,
		reply_to,
		like_count,
		created_at
	FROM photo_comments
	WHERE id = $1
`

func (c *PhotoCommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.PhotoComment, error) {
	pc, err := scanPhotoComment(c.r.conn(ctx).QueryRow(ctx, getPhotoCommentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrCommentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPhotoComment", err)
	}

	return pc, nil
}

const deletePhotoCommentQuery = `
	WITH gone AS (
		DELETE FROM photo_comments
		WHERE id = $1
		RETURNING photo_id
	), uncounted AS (
		UPDATE album_photos p
		SET comment_count = p.comment_count - 1
		FROM gone
		WHERE p.id = gone.photo_id
	)
	SELECT count(*)
	FROM gone
`

func (c *PhotoCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var deleted int

	if err := c.r.conn(ctx).QueryRow(ctx, deletePhotoCommentQuery, id).Scan(&deleted); err != nil {
		return errs.Wrap("c.r.conn.QueryRow", err)
	}

	if deleted == 0 {
		return post.ErrCommentNotFound
	}

	return nil
}

const listPhotoCommentsQuery = `
	SELECT
		id,
		photo_id,
		author_id,
		text,
		reply_to,
		like_count,
		created_at
	FROM photo_comments
	WHERE photo_id = $1
		AND id > $2
		AND NOT user_hidden(author_id)
	ORDER BY id
	LIMIT $3
`

func (c *PhotoCommentRepository) List(ctx context.Context, photoID, after uuid.UUID, limit int) ([]*post.PhotoComment, error) {
	rows, err := c.r.conn(ctx).Query(ctx, listPhotoCommentsQuery, photoID, after, limit)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
	}

	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.PhotoComment, error) {
		return scanPhotoComment(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

const getPhotoCommentsQuery = `
	SELECT
		id,
		photo_id,
		author_id,
		text,
		reply_to,
		like_count,
		created_at
	FROM photo_comments
	WHERE id = ANY($1)
`

func (c *PhotoCommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.PhotoComment, error) {
	rows, err := c.r.conn(ctx).Query(ctx, getPhotoCommentsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
	}

	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.PhotoComment, error) {
		return scanPhotoComment(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

// likePhotoCommentQuery adds the heart and counts it in one statement; a
// repeated like inserts nothing and adds 0.
const likePhotoCommentQuery = `
	WITH liked AS (
		INSERT INTO photo_comment_likes (
			comment_id,
			user_id,
			created_at
		)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING
		RETURNING 1
	)
	UPDATE photo_comments
	SET like_count = like_count + (SELECT count(*) FROM liked)
	WHERE id = $1
	RETURNING like_count
`

func (c *PhotoCommentRepository) Like(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	var likes int

	err := c.r.conn(ctx).QueryRow(ctx, likePhotoCommentQuery, commentID, userID).Scan(&likes)
	if isForeignKeyViolation(err) {
		return 0, post.ErrCommentNotFound
	}
	if err != nil {
		return 0, errs.Wrap("c.r.conn.QueryRow", err)
	}

	return likes, nil
}

const unlikePhotoCommentQuery = `
	WITH unliked AS (
		DELETE FROM photo_comment_likes
		WHERE comment_id = $1
			AND user_id = $2
		RETURNING 1
	)
	UPDATE photo_comments
	SET like_count = like_count - (SELECT count(*) FROM unliked)
	WHERE id = $1
	RETURNING like_count
`

func (c *PhotoCommentRepository) Unlike(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	var likes int

	err := c.r.conn(ctx).QueryRow(ctx, unlikePhotoCommentQuery, commentID, userID).Scan(&likes)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, post.ErrCommentNotFound
	}
	if err != nil {
		return 0, errs.Wrap("c.r.conn.QueryRow", err)
	}

	return likes, nil
}

const listLikedPhotoCommentsQuery = `
	SELECT comment_id
	FROM photo_comment_likes
	WHERE user_id = $1
		AND comment_id = ANY($2)
`

func (c *PhotoCommentRepository) Liked(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := c.r.conn(ctx).Query(ctx, listLikedPhotoCommentsQuery, userID, commentIDs)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
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

const listPhotoCommentLikersQuery = `
	SELECT
		user_id,
		created_at
	FROM photo_comment_likes
	WHERE comment_id = $1
		AND (created_at, user_id) < ($2, $3)
	ORDER BY created_at DESC, user_id DESC
	LIMIT $4
`

func (c *PhotoCommentRepository) Likers(ctx context.Context, commentID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	at, userID := likersFrom(after)

	rows, err := c.r.conn(ctx).Query(ctx, listPhotoCommentLikersQuery, commentID, at, userID, limit)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
	}

	return collectLikers(rows)
}

func scanPhotoComment(row pgx.Row) (*post.PhotoComment, error) {
	var (
		s       post.StoredPhotoComment
		text    string
		replyTo *uuid.UUID
	)

	if err := row.Scan(&s.ID, &s.PhotoID, &s.AuthorID, &text, &replyTo, &s.Likes, &s.CreatedAt); err != nil {
		return nil, err
	}

	if replyTo != nil {
		s.ReplyTo = *replyTo
	}

	// Data comes from our own table and was validated on write.
	s.Text, s.CreatedAt = post.CommentText(text), s.CreatedAt.UTC()

	return post.RestorePhotoComment(s), nil
}

// The likes and comments a user leaves on photos, and their hearts on
// comments there, go when the user is erased, uncounted by the same
// statements.
const (
	deleteUserPhotoCommentLikesQuery = `
	WITH gone AS (
		DELETE FROM photo_comment_likes
		WHERE user_id = $1
		RETURNING comment_id
	)
	UPDATE photo_comments c
	SET like_count = c.like_count - g.n
	FROM (SELECT comment_id, count(*) AS n FROM gone GROUP BY comment_id) g
	WHERE c.id = g.comment_id
`

	deleteUserPhotoLikesQuery = `
	WITH gone AS (
		DELETE FROM album_photo_likes
		WHERE user_id = $1
		RETURNING photo_id
	)
	UPDATE album_photos p
	SET like_count = p.like_count - g.n
	FROM (SELECT photo_id, count(*) AS n FROM gone GROUP BY photo_id) g
	WHERE p.id = g.photo_id
`

	deleteUserPhotoCommentsQuery = `
	WITH gone AS (
		DELETE FROM photo_comments
		WHERE author_id = $1
		RETURNING photo_id
	)
	UPDATE album_photos p
	SET comment_count = p.comment_count - g.n
	FROM (SELECT photo_id, count(*) AS n FROM gone GROUP BY photo_id) g
	WHERE p.id = g.photo_id
`
)
