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

type PostponedRepository struct {
	pool *pgxpool.Pool
}

func NewPostponedRepository(pool *pgxpool.Pool) *PostponedRepository {
	return &PostponedRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *PostponedRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// savePostponedQuery inserts nothing when $12 posts wait for the wall
// already. Two saves at once may both pass: the cap is against piling up,
// not exact.
const savePostponedQuery = `
	INSERT INTO postponed_posts (
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		signed,
		nsfw,
		mentioned_ids,
		publish_at,
		created_at
	)
	SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
	WHERE (
		SELECT count(*)
		FROM postponed_posts
		WHERE wall_id = $2
	) < $12
`

func (r *PostponedRepository) Save(ctx context.Context, p *post.Postponed) error {
	content := p.Content()

	tag, err := r.conn(ctx).Exec(ctx, savePostponedQuery,
		p.ID(), p.WallID(), p.AuthorID(), content.Text().String(), photoIDs(content.Photos()), p.FromWall(), p.Signed(),
		p.NSFW(), mentionIDs(p.Mentions()), p.PublishAt(), p.CreatedAt(), post.MaxPostponed)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrTooManyPostponed
	}

	return nil
}

const getPostponedQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		signed,
		nsfw,
		mentioned_ids,
		publish_at,
		created_at
	FROM postponed_posts
	WHERE id = $1
`

func (r *PostponedRepository) Get(ctx context.Context, id uuid.UUID) (*post.Postponed, error) {
	p, err := scanPostponed(r.conn(ctx).QueryRow(ctx, getPostponedQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrPostponedNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPostponed", err)
	}

	return p, nil
}

const updatePostponedQuery = `
	UPDATE postponed_posts
	SET text = $2,
		nsfw = $3,
		mentioned_ids = $4,
		publish_at = $5
	WHERE id = $1
`

func (r *PostponedRepository) Update(ctx context.Context, p *post.Postponed) error {
	tag, err := r.conn(ctx).Exec(ctx, updatePostponedQuery,
		p.ID(), p.Content().Text().String(), p.NSFW(), mentionIDs(p.Mentions()), p.PublishAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrPostponedNotFound
	}

	return nil
}

const deletePostponedQuery = `
	DELETE FROM postponed_posts
	WHERE id = $1
`

func (r *PostponedRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deletePostponedQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrPostponedNotFound
	}

	return nil
}

const listPostponedQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		signed,
		nsfw,
		mentioned_ids,
		publish_at,
		created_at
	FROM postponed_posts
	WHERE wall_id = $1
	ORDER BY publish_at, id
`

func (r *PostponedRepository) List(ctx context.Context, wallID uuid.UUID) ([]*post.Postponed, error) {
	return r.query(ctx, listPostponedQuery, wallID)
}

const duePostponedQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		signed,
		nsfw,
		mentioned_ids,
		publish_at,
		created_at
	FROM postponed_posts
	WHERE publish_at <= $1
	ORDER BY publish_at, id
	LIMIT $2
`

func (r *PostponedRepository) Due(ctx context.Context, now time.Time, limit int) ([]*post.Postponed, error) {
	return r.query(ctx, duePostponedQuery, now, limit)
}

func (r *PostponedRepository) query(ctx context.Context, query string, args ...any) ([]*post.Postponed, error) {
	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []*post.Postponed
	for rows.Next() {
		p, err := scanPostponed(rows)
		if err != nil {
			return nil, errs.Wrap("scanPostponed", err)
		}
		found = append(found, p)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const deleteUserPostponedQuery = `
	DELETE FROM postponed_posts
	WHERE author_id = $1
`

// DeleteByUser deletes what a user postponed; media erases their photos
// with the rest of the user.
func (r *PostponedRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserPostponedQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// mentionIDs are the people a text names as a column: never NULL.
func mentionIDs(m post.Mentions) []uuid.UUID {
	if m == nil {
		return []uuid.UUID{}
	}

	return m
}

func scanPostponed(row pgx.Row) (*post.Postponed, error) {
	var (
		p        post.StoredPostponed
		text     string
		photos   []uuid.UUID
		mentions []uuid.UUID
	)

	err := row.Scan(&p.ID, &p.WallID, &p.AuthorID, &text, &photos, &p.FromWall, &p.Signed, &p.NSFW, &mentions,
		&p.PublishAt, &p.CreatedAt)
	if err != nil {
		return nil, err
	}

	if len(photos) == 0 {
		photos = nil
	}
	if len(mentions) > 0 {
		p.Mentions = mentions
	}

	// Data comes from our own table and was validated on write.
	p.Content = post.RestoreContent(post.Text(text), post.Photos(photos), uuid.Nil)
	p.PublishAt, p.CreatedAt = p.PublishAt.UTC(), p.CreatedAt.UTC()

	return post.RestorePostponed(p), nil
}
