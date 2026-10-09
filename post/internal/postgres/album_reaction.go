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
			created_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING photo_id
	)
	UPDATE album_photos p
	SET comment_count = p.comment_count + 1
	FROM saved
	WHERE p.id = saved.photo_id
`

func (c *PhotoCommentRepository) Save(ctx context.Context, pc *post.PhotoComment) error {
	_, err := c.r.conn(ctx).Exec(ctx, savePhotoCommentQuery, pc.ID(), pc.PhotoID(), pc.AuthorID(), pc.Text().String(), pc.CreatedAt())
	if isForeignKeyViolation(err) {
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
		created_at
	FROM photo_comments
	WHERE photo_id = $1
		AND id > $2
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

func scanPhotoComment(row pgx.Row) (*post.PhotoComment, error) {
	var (
		s    post.StoredPhotoComment
		text string
	)

	if err := row.Scan(&s.ID, &s.PhotoID, &s.AuthorID, &text, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Text, s.CreatedAt = post.CommentText(text), s.CreatedAt.UTC()

	return post.RestorePhotoComment(s), nil
}

// The likes and comments a user leaves on photos go when the user is
// erased, uncounted in the photos by the same statements.
const (
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
