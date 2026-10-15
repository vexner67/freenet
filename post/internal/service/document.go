package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type DocumentRepository interface {
	Save(ctx context.Context, d *post.Document) error
	// Get, Update and Delete return post.ErrDocumentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Document, error)
	Update(ctx context.Context, d *post.Document) error
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit documents of an owner, newest first, with
	// IDs below before (uuid.Nil: from the newest); Count counts them all.
	List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Document, error)
	Count(ctx context.Context, ownerID uuid.UUID) (int, error)
	// GetMany returns the documents that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Document, error)
}

// DocumentKeeper records, inside the transaction of the change, that the
// media service has to delete the bytes of deleted documents.
type DocumentKeeper interface {
	DeleteDocuments(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error
}

// DocumentPage is one screen of the documents of a person; NextCursor is
// uuid.Nil on the last, Total counts them on all pages.
type DocumentPage struct {
	Documents  []*post.Document
	NextCursor uuid.UUID
	Total      int
}

// NewDocumentInput is a file the media service has kept for OwnerID as ID.
type NewDocumentInput struct {
	ID      uuid.UUID
	OwnerID uuid.UUID
	Name    post.DocumentName
	Ext     post.DocumentExt
	Size    int64
}

// DocumentService keeps "Мои Документы" of openvk: the files of a person,
// which posts and messages attach. Only the owner lists them; who reads an
// attachment is whoever reads what attaches it.
type DocumentService struct {
	documents DocumentRepository
	tx        TxManager
	keeper    DocumentKeeper
	now       func() time.Time
}

func NewDocumentService(documents DocumentRepository, tx TxManager, keeper DocumentKeeper) *DocumentService {
	return &DocumentService{documents: documents, tx: tx, keeper: keeper, now: time.Now}
}

// Add keeps a file the media service has just stored for its owner.
func (s *DocumentService) Add(ctx context.Context, in NewDocumentInput) (*post.Document, error) {
	d, err := post.NewDocument(in.ID, in.OwnerID, in.Name, in.Ext, in.Size, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewDocument", err)
	}

	if err = s.documents.Save(ctx, d); err != nil {
		return nil, errs.Wrap("s.documents.Save", err)
	}

	return d, nil
}

// List pages the documents of ownerID, newest first.
func (s *DocumentService) List(ctx context.Context, ownerID, cursor uuid.UUID, size int) (DocumentPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.documents.List(ctx, ownerID, cursor, size+1)
	if err != nil {
		return DocumentPage{}, errs.Wrap("s.documents.List", err)
	}

	page := DocumentPage{Documents: found}
	if len(found) > size {
		page.Documents = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.documents.Count(ctx, ownerID); err != nil {
		return DocumentPage{}, errs.Wrap("s.documents.Count", err)
	}

	return page, nil
}

// ByID returns the documents with these IDs that exist, at most
// MaxPageSize, in no particular order.
func (s *DocumentService) ByID(ctx context.Context, ids []uuid.UUID) ([]*post.Document, error) {
	if len(ids) > MaxPageSize {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	found, err := s.documents.GetMany(ctx, ids)

	return found, errs.Wrap("s.documents.GetMany", err)
}

// Rename calls a document name: only its owner may.
func (s *DocumentService) Rename(ctx context.Context, requesterID, id uuid.UUID, name post.DocumentName) (*post.Document, error) {
	d, err := s.documents.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.documents.Get", err)
	}

	renamed, err := d.Renamed(requesterID, name)
	if err != nil {
		return nil, errs.Wrap("d.Renamed", err)
	}

	if err = s.documents.Update(ctx, renamed); err != nil {
		return nil, errs.Wrap("s.documents.Update", err)
	}

	return renamed, nil
}

// Delete deletes a document of its owner; the media service deletes its
// bytes after, and posts and messages that attached it lose it.
func (s *DocumentService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	d, err := s.documents.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.documents.Get", err)
	}

	if !d.CanBeChangedBy(requesterID) {
		return post.ErrCannotChangeDocument
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.documents.Delete(ctx, id); err != nil {
			return errs.Wrap("s.documents.Delete", err)
		}

		return errs.Wrap("s.keeper.DeleteDocuments", s.keeper.DeleteDocuments(ctx, d.OwnerID(), []uuid.UUID{id}))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
