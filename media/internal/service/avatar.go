package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/pkg/errs"
)

type AvatarStore interface {
	Save(ctx context.Context, userID, avatarID uuid.UUID, images avatar.Images) error
	// Load returns avatar.ErrNotFound.
	Load(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error)
	// DeleteExcept removes the user's avatars other than keep (uuid.Nil: all).
	DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error
}

type AvatarService struct {
	store AvatarStore
}

func NewAvatarService(store AvatarStore) *AvatarService {
	return &AvatarService{
		store: store,
	}
}

// Upload stores a new avatar and returns its ID. Older avatars stay until
// DeleteExcept: the profile still shows one until it points to the new one.
func (s *AvatarService) Upload(ctx context.Context, userID uuid.UUID, data []byte) (uuid.UUID, error) {
	images, err := avatar.Make(data)
	if err != nil {
		return uuid.Nil, errs.Wrap("avatar.Make", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, errs.Wrap("uuid.NewV7", err)
	}

	if err = s.store.Save(ctx, userID, id, images); err != nil {
		return uuid.Nil, errs.Wrap("s.store.Save", err)
	}

	return id, nil
}

func (s *AvatarService) Get(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error) {
	data, err := s.store.Load(ctx, userID, avatarID, size)
	if err != nil {
		return nil, errs.Wrap("s.store.Load", err)
	}

	return data, nil
}

func (s *AvatarService) DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error {
	if err := s.store.DeleteExcept(ctx, userID, keep); err != nil {
		return errs.Wrap("s.store.DeleteExcept", err)
	}

	return nil
}
