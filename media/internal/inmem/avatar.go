package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/pkg/errs"
)

type AvatarStore struct {
	mu      sync.RWMutex
	avatars map[uuid.UUID]map[uuid.UUID]avatar.Images // by user, then avatar
}

func NewAvatarStore() *AvatarStore {
	return &AvatarStore{
		avatars: make(map[uuid.UUID]map[uuid.UUID]avatar.Images),
	}
}

func (s *AvatarStore) Save(ctx context.Context, userID, avatarID uuid.UUID, images avatar.Images) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.avatars[userID] == nil {
		s.avatars[userID] = make(map[uuid.UUID]avatar.Images)
	}
	s.avatars[userID][avatarID] = images

	return nil
}

func (s *AvatarStore) Load(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	images, ok := s.avatars[userID][avatarID]
	if !ok {
		return nil, avatar.ErrNotFound
	}

	return images.Of(size), nil
}

func (s *AvatarStore) DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.avatars[userID] {
		if id != keep {
			delete(s.avatars[userID], id)
		}
	}

	return nil
}
