package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/pkg/errs"
)

type DocumentStore interface {
	Save(ctx context.Context, ownerID, documentID uuid.UUID, data []byte) error
	// Load returns document.ErrNotFound.
	Load(ctx context.Context, ownerID, documentID uuid.UUID) ([]byte, error)
	// Delete removes the owner's documents; missing ones are fine.
	Delete(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error
	// DeleteAll removes every document of the owner.
	DeleteAll(ctx context.Context, ownerID uuid.UUID) error
}

// DocumentService keeps the files of "Мои Документы". A document is kept
// for good as soon as it is uploaded: the post service, which knows its
// name, adds it to the owner's documents at once, and the gateway deletes
// it here when that fails.
type DocumentService struct {
	store DocumentStore
}

func NewDocumentService(store DocumentStore) *DocumentService {
	return &DocumentService{store: store}
}

// Upload keeps data as a new document of ownerID and returns its ID.
func (s *DocumentService) Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error) {
	if err := document.Check(data); err != nil {
		return uuid.Nil, errs.Wrap("document.Check", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, errs.Wrap("uuid.NewV7", err)
	}

	if err = s.store.Save(ctx, ownerID, id, data); err != nil {
		return uuid.Nil, errs.Wrap("s.store.Save", err)
	}

	return id, nil
}

func (s *DocumentService) Get(ctx context.Context, ownerID, documentID uuid.UUID) ([]byte, error) {
	data, err := s.store.Load(ctx, ownerID, documentID)
	if err != nil {
		return nil, errs.Wrap("s.store.Load", err)
	}

	return data, nil
}

// Delete removes the owner's documents; missing ones are fine.
func (s *DocumentService) Delete(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error {
	return errs.Wrap("s.store.Delete", s.store.Delete(ctx, ownerID, documentIDs))
}
