package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/pkg/errs"
)

type DocumentStore struct {
	mu   sync.RWMutex
	docs map[uuid.UUID]map[uuid.UUID][]byte // by owner, then document
}

func NewDocumentStore() *DocumentStore {
	return &DocumentStore{docs: make(map[uuid.UUID]map[uuid.UUID][]byte)}
}

func (s *DocumentStore) Save(ctx context.Context, ownerID, documentID uuid.UUID, data []byte) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.docs[ownerID] == nil {
		s.docs[ownerID] = make(map[uuid.UUID][]byte)
	}
	s.docs[ownerID][documentID] = append([]byte(nil), data...)

	return nil
}

func (s *DocumentStore) Load(ctx context.Context, ownerID, documentID uuid.UUID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.docs[ownerID][documentID]
	if !ok {
		return nil, document.ErrNotFound
	}

	return data, nil
}

func (s *DocumentStore) Delete(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range documentIDs {
		delete(s.docs[ownerID], id)
	}

	return nil
}

func (s *DocumentStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.docs, ownerID)

	return nil
}
