package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
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

func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	content := p.Content()

	// pgx writes a nil slice as NULL; the column wants an empty array.
	photos := []uuid.UUID(content.Photos())
	if photos == nil {
		photos = []uuid.UUID{}
	}

	_, err := r.pool.Exec(ctx, savePostQuery, p.ID(), p.WallID(), p.AuthorID(), content.Text().String(), photos, p.CreatedAt())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
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
		like_count,
		comment_count
	FROM posts
	WHERE id = $1
`

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	p, err := scanPost(r.pool.QueryRow(ctx, getPostQuery, id))
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
		like_count,
		comment_count
	FROM posts
	WHERE id = ANY($1)
`

func (r *PostRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error) {
	rows, err := r.pool.Query(ctx, getPostsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var posts []*post.Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, errs.Wrap("scanPost", err)
		}
		posts = append(posts, p)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return posts, nil
}

const deletePostQuery = `
	DELETE FROM posts
	WHERE id = $1
`

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deletePostQuery, id)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNotFound
	}

	return nil
}

const listPostsByWallQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		like_count,
		comment_count
	FROM posts
	WHERE wall_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *PostRepository) ListByWall(ctx context.Context, wallID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.list(ctx, listPostsByWallQuery, wallID, before, limit)
}

// list runs a newest-first keyset query; owners is a wall ID or a slice of
// author IDs.
func (r *PostRepository) list(ctx context.Context, query string, owners any, before uuid.UUID, limit int) ([]*post.Post, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.pool.Query(ctx, query, owners, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var posts []*post.Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, errs.Wrap("scanPost", err)
		}
		posts = append(posts, p)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return posts, nil
}

const listPostsByAuthorsQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		like_count,
		comment_count
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

	if err := r.pool.QueryRow(ctx, countPostsByWallQuery, wallID).Scan(&count); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return count, nil
}

func scanPost(row pgx.Row) (*post.Post, error) {
	var (
		id, wallID, authorID uuid.UUID
		text                 string
		photos               []uuid.UUID
		createdAt            time.Time
		counters             post.Counters
	)

	if err := row.Scan(&id, &wallID, &authorID, &text, &photos, &createdAt, &counters.Likes, &counters.Comments); err != nil {
		return nil, err
	}

	if len(photos) == 0 {
		photos = nil // no photos, as a new post has them
	}

	// Data comes from our own table and was validated on write.
	content := post.RestoreContent(post.Text(text), post.Photos(photos))

	return post.Restore(id, wallID, authorID, content, createdAt.UTC(), counters), nil
}
