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

// AlbumRepository keeps the albums of people and communities and their
// photos: it is both service.AlbumRepository and
// service.AlbumPhotoRepository.
type AlbumRepository struct {
	pool *pgxpool.Pool
}

func NewAlbumRepository(pool *pgxpool.Pool) *AlbumRepository {
	return &AlbumRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *AlbumRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// Photos is the same repository as service.AlbumPhotoRepository.
func (r *AlbumRepository) Photos() *AlbumPhotoRepository {
	return &AlbumPhotoRepository{r: r}
}

const saveAlbumQuery = `
	INSERT INTO albums (
		id,
		owner_id,
		name,
		description,
		created_at,
		updated_at,
		kind
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
`

func (r *AlbumRepository) Save(ctx context.Context, a *post.Album) error {
	_, err := r.conn(ctx).Exec(ctx, saveAlbumQuery, a.ID(), a.OwnerID(), a.Name().String(), a.Description().String(),
		a.CreatedAt(), a.UpdatedAt(), string(a.Kind()))
	if constraintOf(err) == "albums_avatars_uidx" {
		return post.ErrAvatarAlbumExists
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getAlbumQuery = `
	SELECT
		id,
		owner_id,
		name,
		description,
		created_at,
		updated_at,
		photo_count,
		kind
	FROM albums
	WHERE id = $1
`

func (r *AlbumRepository) Get(ctx context.Context, id uuid.UUID) (*post.Album, error) {
	a, err := scanAlbum(r.conn(ctx).QueryRow(ctx, getAlbumQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrAlbumNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAlbum", err)
	}

	return a, nil
}

const avatarAlbumQuery = `
	SELECT
		id,
		owner_id,
		name,
		description,
		created_at,
		updated_at,
		photo_count,
		kind
	FROM albums
	WHERE owner_id = $1
		AND kind = 'avatars'
`

func (r *AlbumRepository) AvatarAlbum(ctx context.Context, ownerID uuid.UUID) (*post.Album, error) {
	a, err := scanAlbum(r.conn(ctx).QueryRow(ctx, avatarAlbumQuery, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrAlbumNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAlbum", err)
	}

	return a, nil
}

const updateAlbumQuery = `
	UPDATE albums
	SET name = $2,
		description = $3,
		updated_at = $4
	WHERE id = $1
`

func (r *AlbumRepository) Update(ctx context.Context, a *post.Album) error {
	tag, err := r.conn(ctx).Exec(ctx, updateAlbumQuery, a.ID(), a.Name().String(), a.Description().String(), a.UpdatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAlbumNotFound
	}

	return nil
}

// lockAlbumQuery makes photos added meanwhile wait (their statement
// updates the album's row) and then fail, rather than go unseen with the
// album and leave their images behind.
const lockAlbumQuery = `
	SELECT id
	FROM albums
	WHERE id = $1
	FOR UPDATE
`

// deleteAlbumQuery returns the photos the album takes with it: the
// statement sees them as they were before the delete.
const deleteAlbumQuery = `
	WITH gone AS (
		DELETE FROM albums
		WHERE id = $1
		RETURNING id
	)
	SELECT
		id,
		album_id,
		author_id,
		description,
		created_at
	FROM album_photos
	WHERE album_id IN (SELECT id FROM gone)
`

// Delete runs in the transaction of ctx, which holds the lock it takes.
func (r *AlbumRepository) Delete(ctx context.Context, id uuid.UUID) ([]*post.AlbumPhoto, error) {
	var locked uuid.UUID
	err := r.conn(ctx).QueryRow(ctx, lockAlbumQuery, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrAlbumNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	rows, err := r.conn(ctx).Query(ctx, deleteAlbumQuery, id)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectAlbumPhotos(rows)
}

const listAlbumsQuery = `
	SELECT
		id,
		owner_id,
		name,
		description,
		created_at,
		updated_at,
		photo_count,
		kind
	FROM albums
	WHERE owner_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *AlbumRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Album, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listAlbumsQuery, ownerID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []*post.Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, errs.Wrap("scanAlbum", err)
		}
		found = append(found, a)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const countAlbumsQuery = `
	SELECT count(*)
	FROM albums
	WHERE owner_id = $1
`

func (r *AlbumRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countAlbumsQuery, ownerID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const deleteUserAlbumsQuery = `
	DELETE FROM albums
	WHERE owner_id = $1
`

// deleteUserAlbumPhotosQuery deletes the photos a user put in others'
// albums and uncounts them in one statement.
const deleteUserAlbumPhotosQuery = `
	WITH deleted AS (
		DELETE FROM album_photos
		WHERE author_id = $1
		RETURNING album_id
	)
	UPDATE albums a
	SET photo_count = a.photo_count - d.n
	FROM (
		SELECT album_id, count(*) AS n
		FROM deleted
		GROUP BY album_id
	) d
	WHERE a.id = d.album_id
`

// DeleteByUser deletes the albums a user owns with their photos, and the
// photos they put in others' albums. It runs in the transaction of ctx.
func (r *AlbumRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserAlbumsQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if _, err := r.conn(ctx).Exec(ctx, deleteUserAlbumPhotosQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func scanAlbum(row pgx.Row) (*post.Album, error) {
	var (
		s                 post.StoredAlbum
		name, description string
	)

	var kind string
	if err := row.Scan(&s.ID, &s.OwnerID, &name, &description, &s.CreatedAt, &s.UpdatedAt, &s.Photos, &kind); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Name, s.Description, s.Kind = post.AlbumName(name), post.Description(description), post.AlbumKind(kind)
	s.CreatedAt, s.UpdatedAt = s.CreatedAt.UTC(), s.UpdatedAt.UTC()

	return post.RestoreAlbum(s), nil
}

// AlbumPhotoRepository is the service.AlbumPhotoRepository side of
// AlbumRepository.
type AlbumPhotoRepository struct {
	r *AlbumRepository
}

// addAlbumPhotosQuery counts the photos in their album and dates it in
// one statement; nothing is added when the album is gone.
const addAlbumPhotosQuery = `
	WITH album AS (
		UPDATE albums
		SET photo_count = photo_count + cardinality($2::uuid[]),
			updated_at = $6
		WHERE id = $1
		RETURNING id
	)
	INSERT INTO album_photos (
		id,
		album_id,
		author_id,
		description,
		created_at
	)
	SELECT p.id, album.id, p.author_id, p.description, p.created_at
	FROM album, unnest($2::uuid[], $3::uuid[], $4::text[], $5::timestamptz[]) AS p (id, author_id, description, created_at)
`

func (p *AlbumPhotoRepository) Add(ctx context.Context, albumID uuid.UUID, photos []*post.AlbumPhoto, now time.Time) error {
	var (
		ids          = make([]uuid.UUID, len(photos))
		authors      = make([]uuid.UUID, len(photos))
		descriptions = make([]string, len(photos))
		createdAt    = make([]time.Time, len(photos))
	)
	for i, photo := range photos {
		ids[i], authors[i], descriptions[i], createdAt[i] = photo.ID(), photo.AuthorID(), photo.Description().String(), photo.CreatedAt()
	}

	tag, err := p.r.conn(ctx).Exec(ctx, addAlbumPhotosQuery, albumID, ids, authors, descriptions, createdAt,
		now.UTC().Truncate(time.Microsecond))
	if constraintOf(err) == "album_photos_pkey" {
		return post.ErrDuplicatePhoto
	}
	if err != nil {
		return errs.Wrap("p.r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAlbumNotFound
	}

	return nil
}

const getAlbumPhotoQuery = `
	SELECT
		id,
		album_id,
		author_id,
		description,
		created_at
	FROM album_photos
	WHERE id = $1
`

func (p *AlbumPhotoRepository) Get(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, error) {
	photo, err := scanAlbumPhoto(p.r.conn(ctx).QueryRow(ctx, getAlbumPhotoQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrAlbumPhotoNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAlbumPhoto", err)
	}

	return photo, nil
}

const describeAlbumPhotoQuery = `
	UPDATE album_photos
	SET description = $2
	WHERE id = $1
`

func (p *AlbumPhotoRepository) Describe(ctx context.Context, photo *post.AlbumPhoto) error {
	tag, err := p.r.conn(ctx).Exec(ctx, describeAlbumPhotoQuery, photo.ID(), photo.Description().String())
	if err != nil {
		return errs.Wrap("p.r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAlbumPhotoNotFound
	}

	return nil
}

// deleteAlbumPhotoQuery removes a photo, uncounts it and dates its album
// in one statement.
const deleteAlbumPhotoQuery = `
	WITH deleted AS (
		DELETE FROM album_photos
		WHERE id = $1
		RETURNING album_id
	)
	UPDATE albums
	SET photo_count = photo_count - 1,
		updated_at = $2
	FROM deleted
	WHERE albums.id = deleted.album_id
`

func (p *AlbumPhotoRepository) Delete(ctx context.Context, id uuid.UUID, now time.Time) error {
	tag, err := p.r.conn(ctx).Exec(ctx, deleteAlbumPhotoQuery, id, now.UTC().Truncate(time.Microsecond))
	if err != nil {
		return errs.Wrap("p.r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAlbumPhotoNotFound
	}

	return nil
}

const listAlbumPhotosQuery = `
	SELECT
		id,
		album_id,
		author_id,
		description,
		created_at
	FROM album_photos
	WHERE album_id = $1
		AND id > $2
	ORDER BY id
	LIMIT $3
`

func (p *AlbumPhotoRepository) List(ctx context.Context, albumID, after uuid.UUID, limit int) ([]*post.AlbumPhoto, error) {
	rows, err := p.r.conn(ctx).Query(ctx, listAlbumPhotosQuery, albumID, after, limit)
	if err != nil {
		return nil, errs.Wrap("p.r.conn.Query", err)
	}

	return collectAlbumPhotos(rows)
}

const albumCoversQuery = `
	SELECT DISTINCT ON (album_id)
		id,
		album_id,
		author_id,
		description,
		created_at
	FROM album_photos
	WHERE album_id = ANY($1)
	ORDER BY album_id, id
`

func (p *AlbumPhotoRepository) Covers(ctx context.Context, albumIDs []uuid.UUID) ([]*post.AlbumPhoto, error) {
	rows, err := p.r.conn(ctx).Query(ctx, albumCoversQuery, albumIDs)
	if err != nil {
		return nil, errs.Wrap("p.r.conn.Query", err)
	}

	return collectAlbumPhotos(rows)
}

// collectAlbumPhotos reads the photos of rows and closes them.
func collectAlbumPhotos(rows pgx.Rows) ([]*post.AlbumPhoto, error) {
	defer rows.Close()

	var found []*post.AlbumPhoto
	for rows.Next() {
		photo, err := scanAlbumPhoto(rows)
		if err != nil {
			return nil, errs.Wrap("scanAlbumPhoto", err)
		}
		found = append(found, photo)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

func scanAlbumPhoto(row pgx.Row) (*post.AlbumPhoto, error) {
	var (
		s           post.StoredAlbumPhoto
		description string
	)

	if err := row.Scan(&s.ID, &s.AlbumID, &s.AuthorID, &description, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Description = post.Description(description)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreAlbumPhoto(s), nil
}
