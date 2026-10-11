package inmem

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

type PhotoStore struct {
	mu     sync.RWMutex
	photos map[uuid.UUID]map[uuid.UUID]storedPhoto // by owner, then photo
}

type storedPhoto struct {
	images    photo.Images
	published bool
	// retired: set aside as unpublished too long; only Publish finds it.
	retired bool
}

func NewPhotoStore() *PhotoStore {
	return &PhotoStore{
		photos: make(map[uuid.UUID]map[uuid.UUID]storedPhoto),
	}
}

func (s *PhotoStore) Save(ctx context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.photos[ownerID] == nil {
		s.photos[ownerID] = make(map[uuid.UUID]storedPhoto)
	}
	s.photos[ownerID][photoID] = storedPhoto{images: images}

	return nil
}

func (s *PhotoStore) Load(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.photos[ownerID][photoID]
	if !ok || p.retired || (size != photo.Large && size != photo.Small) {
		return nil, photo.ErrNotFound
	}

	return p.images.Of(size), nil
}

func (s *PhotoStore) Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var found []uuid.UUID
	for _, id := range photoIDs {
		if p, ok := s.photos[ownerID][id]; ok && !p.retired {
			found = append(found, id)
		}
	}

	return found, nil
}

func (s *PhotoStore) Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range photoIDs {
		if p, ok := s.photos[ownerID][id]; ok {
			p.published, p.retired = true, false
			s.photos[ownerID][id] = p
		}
	}

	return nil
}

func (s *PhotoStore) Copy(ctx context.Context, ownerID, from, to uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.photos[ownerID][from]
	if _, copied := s.photos[ownerID][to]; !ok || p.retired || copied {
		return nil
	}

	s.photos[ownerID][to] = storedPhoto{images: p.images, published: true}

	return nil
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

func (s *PhotoStore) DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, p := range s.photos[ownerID] {
		if p.published && !slices.Contains(keep, id) {
			delete(s.photos[ownerID], id)
		}
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

// Retire goes by the time in the UUIDv7 of a photo, as fs does.
func (s *PhotoStore) Retire(ctx context.Context, before time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	retired := 0
	for _, photos := range s.photos {
		for id, p := range photos {
			if !p.published && !p.retired && time.Unix(id.Time().UnixTime()).Before(before) {
				p.retired = true
				photos[id] = p
				retired++
			}
		}
	}

	return retired, nil
}

func (s *PhotoStore) DeleteRetired(ctx context.Context, before time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	deleted := 0
	for ownerID, photos := range s.photos {
		for id, p := range photos {
			if p.retired && time.Unix(id.Time().UnixTime()).Before(before) {
				delete(photos, id)
				deleted++
			}
		}

		if len(photos) == 0 {
			delete(s.photos, ownerID)
		}
	}

	return deleted, nil
}
