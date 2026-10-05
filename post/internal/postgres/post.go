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
		author_id,
		text,
		created_at
	)
	VALUES ($1, $2, $3, $4)
`

func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	if _, err := r.pool.Exec(ctx, savePostQuery, p.ID(), p.AuthorID(), p.Text().String(), p.CreatedAt()); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const getPostQuery = `
	SELECT
		id,
		author_id,
		text,
		created_at
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

const listPostsByAuthorQuery = `
	SELECT
		id,
		author_id,
		text,
		created_at
	FROM posts
	WHERE author_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *PostRepository) ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.list(ctx, listPostsByAuthorQuery, authorID, before, limit)
}

// list runs a newest-first keyset query; authors is one ID or a slice.
func (r *PostRepository) list(ctx context.Context, query string, authors any, before uuid.UUID, limit int) ([]*post.Post, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.pool.Query(ctx, query, authors, before, limit)
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
		author_id,
		text,
		created_at
	FROM posts
	WHERE author_id = ANY($1)
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *PostRepository) ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.list(ctx, listPostsByAuthorsQuery, authorIDs, before, limit)
}

const countPostsByAuthorQuery = `
	SELECT count(*)
	FROM posts
	WHERE author_id = $1
`

func (r *PostRepository) CountByAuthor(ctx context.Context, authorID uuid.UUID) (int, error) {
	var count int

	if err := r.pool.QueryRow(ctx, countPostsByAuthorQuery, authorID).Scan(&count); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return count, nil
}

func scanPost(row pgx.Row) (*post.Post, error) {
	var (
		id, authorID uuid.UUID
		text         string
		createdAt    time.Time
	)

	if err := row.Scan(&id, &authorID, &text, &createdAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	return post.Restore(id, authorID, post.Text(text), createdAt.UTC()), nil
}
