package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/audio"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/pkg/errs"
)

// FileStore keeps documents or audio in memory.
type FileStore struct {
	mu   sync.RWMutex
	docs map[uuid.UUID]map[uuid.UUID][]byte // by owner, then file
	// missing is what Load returns for a file that is not there.
	missing error
}

func NewDocumentStore() *FileStore {
	return &FileStore{docs: make(map[uuid.UUID]map[uuid.UUID][]byte), missing: document.ErrNotFound}
}

func NewAudioStore() *FileStore {
	return &FileStore{docs: make(map[uuid.UUID]map[uuid.UUID][]byte), missing: audio.ErrNotFound}
}

func (s *FileStore) Save(ctx context.Context, ownerID, fileID uuid.UUID, data []byte) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.docs[ownerID] == nil {
		s.docs[ownerID] = make(map[uuid.UUID][]byte)
	}
	s.docs[ownerID][fileID] = append([]byte(nil), data...)

	return nil
}

func (s *FileStore) Load(ctx context.Context, ownerID, fileID uuid.UUID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.docs[ownerID][fileID]
	if !ok {
		return nil, s.missing
	}

	return data, nil
}

func (s *FileStore) Delete(ctx context.Context, ownerID uuid.UUID, fileIDs []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range fileIDs {
		delete(s.docs[ownerID], id)
	}

	return nil
}

func (s *FileStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.docs, ownerID)

	return nil
}
