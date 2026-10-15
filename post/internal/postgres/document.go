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

// DocumentRepository keeps the documents of people.
type DocumentRepository struct {
	pool *pgxpool.Pool
}

func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *DocumentRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveDocumentQuery = `
	INSERT INTO documents (
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *DocumentRepository) Save(ctx context.Context, d *post.Document) error {
	_, err := r.conn(ctx).Exec(ctx, saveDocumentQuery, d.ID(), d.OwnerID(), d.Name().String(), d.Ext().String(), d.Size(), d.CreatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getDocumentQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE id = $1
`

func (r *DocumentRepository) Get(ctx context.Context, id uuid.UUID) (*post.Document, error) {
	d, err := scanDocument(r.conn(ctx).QueryRow(ctx, getDocumentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrDocumentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanDocument", err)
	}

	return d, nil
}

const getDocumentsQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE id = ANY($1)
`

func (r *DocumentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Document, error) {
	rows, err := r.conn(ctx).Query(ctx, getDocumentsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectDocuments(rows)
}

const updateDocumentQuery = `
	UPDATE documents
	SET name = $2
	WHERE id = $1
`

func (r *DocumentRepository) Update(ctx context.Context, d *post.Document) error {
	tag, err := r.conn(ctx).Exec(ctx, updateDocumentQuery, d.ID(), d.Name().String())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrDocumentNotFound
	}

	return nil
}

const deleteDocumentQuery = `
	DELETE FROM documents
	WHERE id = $1
`

func (r *DocumentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteDocumentQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrDocumentNotFound
	}

	return nil
}

const listDocumentsQuery = `
	SELECT
		id,
		owner_id,
		name,
		ext,
		size,
		created_at
	FROM documents
	WHERE owner_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *DocumentRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Document, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listDocumentsQuery, ownerID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectDocuments(rows)
}

const countDocumentsQuery = `
	SELECT count(*)
	FROM documents
	WHERE owner_id = $1
`

func (r *DocumentRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countDocumentsQuery, ownerID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

const deleteUserDocumentsQuery = `
	DELETE FROM documents
	WHERE owner_id = $1
`

// DeleteByUser deletes the documents of a user.
func (r *DocumentRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserDocumentsQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func collectDocuments(rows pgx.Rows) ([]*post.Document, error) {
	documents, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Document, error) {
		return scanDocument(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return documents, nil
}

func scanDocument(row pgx.Row) (*post.Document, error) {
	var (
		s    post.StoredDocument
		name string
		ext  string
	)

	if err := row.Scan(&s.ID, &s.OwnerID, &name, &ext, &s.Size, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Name, s.Ext = post.DocumentName(name), post.DocumentExt(ext)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreDocument(s), nil
}
