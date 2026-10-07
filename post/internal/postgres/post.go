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

// newestCursor is above every UUID, so "id < newestCursor" starts a wall
// from its newest post.
var newestCursor = uuid.Max

type PostRepository struct {
	pool *pgxpool.Pool
}

func NewPostRepository(pool *pgxpool.Pool) *PostRepository {
	return &PostRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *PostRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const savePostQuery = `
	INSERT INTO posts (
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

// saveRepostQuery counts the repost in the shared post and adds it in one
// statement; nothing is added when the shared post is gone.
const saveRepostQuery = `
	WITH shared AS (
		UPDATE posts
		SET repost_count = repost_count + 1
		WHERE id = $7
		RETURNING id
	)
	INSERT INTO posts (
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of
	)
	SELECT $1, $2, $3, $4, $5, $6, shared.id
	FROM shared
`

func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	content := p.Content()

	args := []any{p.ID(), p.WallID(), p.AuthorID(), content.Text().String(), photoIDs(content.Photos()), p.CreatedAt()}

	if content.RepostOf() == uuid.Nil {
		if _, err := r.conn(ctx).Exec(ctx, savePostQuery, args...); err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}

		return nil
	}

	tag, err := r.conn(ctx).Exec(ctx, saveRepostQuery, append(args, content.RepostOf())...)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNotFound
	}

	return nil
}

const getPostQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	FROM posts
	WHERE id = $1
`

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	p, err := scanPost(r.conn(ctx).QueryRow(ctx, getPostQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPost", err)
	}

	return p, nil
}

const getPostsQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	FROM posts
	WHERE id = ANY($1)
`

func (r *PostRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error) {
	rows, err := r.conn(ctx).Query(ctx, getPostsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectPosts(rows)
}

// deletePostQuery deletes a post and uncounts it in the post it shared,
// if any and still there, in one statement; it returns how many posts it
// deleted.
const deletePostQuery = `
	WITH deleted AS (
		DELETE FROM posts
		WHERE id = $1
		RETURNING repost_of
	), uncounted AS (
		UPDATE posts
		SET repost_count = repost_count - 1
		WHERE id = (SELECT repost_of FROM deleted)
	)
	SELECT count(*)
	FROM deleted
`

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var deleted int

	if err := r.conn(ctx).QueryRow(ctx, deletePostQuery, id).Scan(&deleted); err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	if deleted == 0 {
		return post.ErrNotFound
	}

	return nil
}

const updatePostQuery = `
	UPDATE posts
	SET text = $2,
		photo_ids = $3,
		edited_at = $4
	WHERE id = $1
`

// Update saves what an edited post says.
func (r *PostRepository) Update(ctx context.Context, p *post.Post) error {
	content := p.Content()

	tag, err := r.conn(ctx).Exec(ctx, updatePostQuery, p.ID(), content.Text().String(), photoIDs(content.Photos()), p.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNotFound
	}

	return nil
}

// pinPostQuery pins a post on its own wall, replacing the post pinned
// there before; nothing when the post is gone.
const pinPostQuery = `
	INSERT INTO pinned_posts (
		wall_id,
		post_id
	)
	SELECT wall_id, id
	FROM posts
	WHERE id = $1
	ON CONFLICT (wall_id) DO UPDATE
	SET post_id = EXCLUDED.post_id
`

func (r *PostRepository) Pin(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, pinPostQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNotFound
	}

	return nil
}

const unpinPostQuery = `
	DELETE FROM pinned_posts
	WHERE post_id = $1
`

func (r *PostRepository) Unpin(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, unpinPostQuery, id); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getPinnedPostQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		true AS pinned
	FROM posts
	WHERE id = (SELECT post_id FROM pinned_posts WHERE wall_id = $1)
`

func (r *PostRepository) Pinned(ctx context.Context, wallID uuid.UUID) (*post.Post, error) {
	p, err := scanPost(r.conn(ctx).QueryRow(ctx, getPinnedPostQuery, wallID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPost", err)
	}

	return p, nil
}

const listPostsByWallQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	FROM posts
	WHERE wall_id = $1
		AND id < $2
		AND id IS DISTINCT FROM (SELECT post_id FROM pinned_posts WHERE wall_id = $1)
	ORDER BY id DESC
	LIMIT $3
`

// ListByWall leaves out the pinned post, which the wall shows first.
func (r *PostRepository) ListByWall(ctx context.Context, wallID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.list(ctx, listPostsByWallQuery, wallID, before, limit)
}

// list runs a newest-first keyset query; owners is a wall ID or a slice of
// author IDs.
func (r *PostRepository) list(ctx context.Context, query string, owners any, before uuid.UUID, limit int) ([]*post.Post, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, query, owners, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectPosts(rows)
}

const listPostsByAuthorsQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	FROM posts
	WHERE author_id = ANY($1)
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *PostRepository) ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.list(ctx, listPostsByAuthorsQuery, authorIDs, before, limit)
}

const countPostsByWallQuery = `
	SELECT count(*)
	FROM posts
	WHERE wall_id = $1
`

func (r *PostRepository) CountByWall(ctx context.Context, wallID uuid.UUID) (int, error) {
	var count int

	if err := r.conn(ctx).QueryRow(ctx, countPostsByWallQuery, wallID).Scan(&count); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return count, nil
}

// photoIDs is photos for the photo_ids column: pgx writes a nil slice as
// NULL, the column wants an empty array.
func photoIDs(photos post.Photos) []uuid.UUID {
	if photos == nil {
		return []uuid.UUID{}
	}

	return photos
}

// collectPosts reads the posts of rows and closes them.
func collectPosts(rows pgx.Rows) ([]*post.Post, error) {
	defer rows.Close()

	var posts []*post.Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, errs.Wrap("scanPost", err)
		}
		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return posts, nil
}

func scanPost(row pgx.Row) (*post.Post, error) {
	var (
		s        post.Stored
		text     string
		photos   []uuid.UUID
		repostOf *uuid.UUID
		editedAt *time.Time
	)

	err := row.Scan(&s.ID, &s.WallID, &s.AuthorID, &text, &photos, &s.CreatedAt, &repostOf,
		&s.Counters.Likes, &s.Counters.Comments, &s.Counters.Reposts, &editedAt, &s.Pinned)
	if err != nil {
		return nil, err
	}

	if len(photos) == 0 {
		photos = nil // no photos, as a new post has them
	}

	// Data comes from our own table and was validated on write.
	shared := uuid.Nil
	if repostOf != nil {
		shared = *repostOf
	}

	s.Content = post.RestoreContent(post.Text(text), post.Photos(photos), shared)
	s.CreatedAt = s.CreatedAt.UTC()
	if editedAt != nil {
		s.EditedAt = editedAt.UTC()
	}

	return post.Restore(s), nil
}

// deletePostsByUserQuery deletes the posts a user wrote and those on their
// wall, with their likes and comments, and uncounts the deleted reposts in
// the posts they shared. A row cannot be both deleted and updated by one
// statement, so shared posts that go too are left alone.
const deletePostsByUserQuery = `
	WITH deleted AS (
		DELETE FROM posts
		WHERE author_id = $1
			OR wall_id = $1
		RETURNING
			id,
			wall_id,
			author_id,
			text,
			photo_ids,
			created_at,
			repost_of,
			like_count,
			comment_count,
			repost_count,
			edited_at,
			EXISTS (SELECT 1 FROM pinned_posts pp WHERE pp.post_id = posts.id) AS pinned
	), uncounted AS (
		UPDATE posts p
		SET repost_count = p.repost_count - d.n
		FROM (
			SELECT repost_of, count(*) AS n
			FROM deleted
			WHERE repost_of IS NOT NULL
			GROUP BY repost_of
		) d
		WHERE p.id = d.repost_of
			AND p.author_id <> $1
			AND p.wall_id <> $1
	)
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		pinned
	FROM deleted
`

func (r *PostRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.Post, error) {
	rows, err := r.conn(ctx).Query(ctx, deletePostsByUserQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectPosts(rows)
}
