package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
)

// ArticleRepository keeps the articles of people, their blocks as jsonb.
type ArticleRepository struct {
	pool *pgxpool.Pool
}

func NewArticleRepository(pool *pgxpool.Pool) *ArticleRepository {
	return &ArticleRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *ArticleRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedBlock and storedSpan are a block of an article as the column keeps
// it: short keys, as the marks repeat in every span.
type storedBlock struct {
	Kind    string       `json:"kind"`
	Spans   []storedSpan `json:"spans,omitempty"`
	PhotoID *uuid.UUID   `json:"photo,omitempty"`
	Caption string       `json:"caption,omitempty"`
}

type storedSpan struct {
	Text   string `json:"text"`
	Bold   bool   `json:"b,omitempty"`
	Italic bool   `json:"i,omitempty"`
	Strike bool   `json:"s,omitempty"`
	Link   string `json:"link,omitempty"`
}

func encodeBlocks(blocks []post.Block) ([]byte, error) {
	stored := make([]storedBlock, 0, len(blocks))
	for _, b := range blocks {
		s := storedBlock{Kind: string(b.Kind), PhotoID: nullID(b.PhotoID), Caption: b.Caption}
		for _, sp := range b.Spans {
			s.Spans = append(s.Spans, storedSpan(sp))
		}
		stored = append(stored, s)
	}

	data, err := json.Marshal(stored)

	return data, errs.Wrap("json.Marshal", err)
}

func decodeBlocks(data []byte) ([]post.Block, error) {
	var stored []storedBlock
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	blocks := make([]post.Block, 0, len(stored))
	for _, s := range stored {
		// Data comes from our own table and was validated on write.
		b := post.Block{Kind: post.BlockKind(s.Kind), Caption: s.Caption}
		if s.PhotoID != nil {
			b.PhotoID = *s.PhotoID
		}
		for _, sp := range s.Spans {
			b.Spans = append(b.Spans, post.Span(sp))
		}
		blocks = append(blocks, b)
	}

	return blocks, nil
}

const saveArticleQuery = `
	INSERT INTO articles (
		id,
		author_id,
		title,
		cover_id,
		blocks,
		created_at,
		edited_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
`

func (r *ArticleRepository) Save(ctx context.Context, a *post.Article) error {
	blocks, err := encodeBlocks(a.Blocks())
	if err != nil {
		return errs.Wrap("encodeBlocks", err)
	}

	_, err = r.conn(ctx).Exec(ctx, saveArticleQuery, a.ID(), a.AuthorID(), a.Title(), nullID(a.CoverID()), blocks, a.CreatedAt(), a.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getArticleQuery = `
	SELECT
		id,
		author_id,
		title,
		cover_id,
		blocks,
		created_at,
		edited_at
	FROM articles
	WHERE id = $1
`

func (r *ArticleRepository) Get(ctx context.Context, id uuid.UUID) (*post.Article, error) {
	a, err := scanArticle(r.conn(ctx).QueryRow(ctx, getArticleQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrArticleNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanArticle", err)
	}

	return a, nil
}

const updateArticleQuery = `
	UPDATE articles
	SET title = $2,
		cover_id = $3,
		blocks = $4,
		edited_at = $5
	WHERE id = $1
`

func (r *ArticleRepository) Update(ctx context.Context, a *post.Article) error {
	blocks, err := encodeBlocks(a.Blocks())
	if err != nil {
		return errs.Wrap("encodeBlocks", err)
	}

	tag, err := r.conn(ctx).Exec(ctx, updateArticleQuery, a.ID(), a.Title(), nullID(a.CoverID()), blocks, a.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrArticleNotFound
	}

	return nil
}

const deleteArticleQuery = `
	DELETE FROM articles
	WHERE id = $1
`

func (r *ArticleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteArticleQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrArticleNotFound
	}

	return nil
}

const listArticlesQuery = `
	SELECT
		id,
		author_id,
		title,
		cover_id,
		blocks,
		created_at,
		edited_at
	FROM articles
	WHERE author_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *ArticleRepository) List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Article, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listArticlesQuery, authorID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	articles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Article, error) {
		return scanArticle(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return articles, nil
}

const countArticlesQuery = `
	SELECT count(*)
	FROM articles
	WHERE author_id = $1
`

func (r *ArticleRepository) Count(ctx context.Context, authorID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countArticlesQuery, authorID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const deleteUserArticlesQuery = `
	DELETE FROM articles
	WHERE author_id = $1
`

// DeleteByUser deletes the articles of a user.
func (r *ArticleRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserArticlesQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func scanArticle(row pgx.Row) (*post.Article, error) {
	var (
		s      post.StoredArticle
		cover  *uuid.UUID
		blocks []byte
	)

	if err := row.Scan(&s.ID, &s.AuthorID, &s.Title, &cover, &blocks, &s.CreatedAt, &s.EditedAt); err != nil {
		return nil, err
	}

	var err error
	if s.Blocks, err = decodeBlocks(blocks); err != nil {
		return nil, errs.Wrap("decodeBlocks", err)
	}

	if cover != nil {
		s.CoverID = *cover
	}

	s.CreatedAt = s.CreatedAt.UTC()
	if s.EditedAt != nil {
		at := s.EditedAt.UTC()
		s.EditedAt = &at
	}

	return post.RestoreArticle(s), nil
}
