package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
)

type SuggestionRepository struct {
	pool *pgxpool.Pool
}

func NewSuggestionRepository(pool *pgxpool.Pool) *SuggestionRepository {
	return &SuggestionRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *SuggestionRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveSuggestionQuery = `
	INSERT INTO suggestions (
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *SuggestionRepository) Save(ctx context.Context, s *post.Suggestion) error {
	content := s.Content()

	_, err := r.conn(ctx).Exec(ctx, saveSuggestionQuery,
		s.ID(), s.WallID(), s.AuthorID(), content.Text().String(), photoIDs(content.Photos()), s.CreatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getSuggestionQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at
	FROM suggestions
	WHERE id = $1
`

func (r *SuggestionRepository) Get(ctx context.Context, id uuid.UUID) (*post.Suggestion, error) {
	s, err := scanSuggestion(r.conn(ctx).QueryRow(ctx, getSuggestionQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrSuggestionNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanSuggestion", err)
	}

	return s, nil
}

const deleteSuggestionQuery = `
	DELETE FROM suggestions
	WHERE id = $1
`

func (r *SuggestionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteSuggestionQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrSuggestionNotFound
	}

	return nil
}

// listSuggestionsQuery: $2 is an author, or NULL for all.
const listSuggestionsQuery = `
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at
	FROM suggestions
	WHERE wall_id = $1
		AND ($2::uuid IS NULL OR author_id = $2)
		AND id < $3
	ORDER BY id DESC
	LIMIT $4
`

func (r *SuggestionRepository) List(ctx context.Context, wallID, authorID, before uuid.UUID, limit int) ([]*post.Suggestion, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listSuggestionsQuery, wallID, optionalAuthor(authorID), before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []*post.Suggestion
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, errs.Wrap("scanSuggestion", err)
		}
		found = append(found, s)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const countSuggestionsQuery = `
	SELECT count(*)
	FROM suggestions
	WHERE wall_id = $1
		AND ($2::uuid IS NULL OR author_id = $2)
`

func (r *SuggestionRepository) Count(ctx context.Context, wallID, authorID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countSuggestionsQuery, wallID, optionalAuthor(authorID)).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const deleteUserSuggestionsQuery = `
	DELETE FROM suggestions
	WHERE author_id = $1
`

// DeleteByUser deletes what a user suggested; media erases their photos
// with the rest of the user.
func (r *SuggestionRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserSuggestionsQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// optionalAuthor is NULL for uuid.Nil: the suggestions of everyone.
func optionalAuthor(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func scanSuggestion(row pgx.Row) (*post.Suggestion, error) {
	var (
		s      post.StoredSuggestion
		text   string
		photos []uuid.UUID
	)

	if err := row.Scan(&s.ID, &s.WallID, &s.AuthorID, &text, &photos, &s.CreatedAt); err != nil {
		return nil, err
	}

	if len(photos) == 0 {
		photos = nil
	}

	// Data comes from our own table and was validated on write.
	s.Content = post.RestoreContent(post.Text(text), post.Photos(photos), uuid.Nil)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreSuggestion(s), nil
}
