package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

type PhotoStore struct {
	mu     sync.RWMutex
	photos map[uuid.UUID]map[uuid.UUID]photo.Images // by owner, then photo
}

func NewPhotoStore() *PhotoStore {
	return &PhotoStore{
		photos: make(map[uuid.UUID]map[uuid.UUID]photo.Images),
	}
}

func (s *PhotoStore) Save(ctx context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.photos[ownerID] == nil {
		s.photos[ownerID] = make(map[uuid.UUID]photo.Images)
	}
	s.photos[ownerID][photoID] = images

	return nil
}

func (s *PhotoStore) Load(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	images, ok := s.photos[ownerID][photoID]
	if !ok || (size != photo.Large && size != photo.Small) {
		return nil, photo.ErrNotFound
	}

	return images.Of(size), nil
}

func (s *PhotoStore) Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var found []uuid.UUID
	for _, id := range photoIDs {
		if _, ok := s.photos[ownerID][id]; ok {
			found = append(found, id)
		}
	}

	return found, nil
}

func (s *PhotoStore) Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range photoIDs {
		delete(s.photos[ownerID], id)
	}

	return nil
}

func (s *PhotoStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.photos, ownerID)

	return nil
}
