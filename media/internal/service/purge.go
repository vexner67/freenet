package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// PurgeService erases the files of a user whose page was deleted for good.
type PurgeService struct {
	avatars AvatarStore
	photos  PhotoStore
}

func NewPurgeService(avatars AvatarStore, photos PhotoStore) *PurgeService {
	return &PurgeService{
		avatars: avatars,
		photos:  photos,
	}
}

// PurgeUser removes all avatars and photos of userID; nothing left is fine.
func (s *PurgeService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.avatars.DeleteExcept(ctx, userID, uuid.Nil); err != nil {
		return errs.Wrap("s.avatars.DeleteExcept", err)
	}

	if err := s.photos.DeleteAll(ctx, userID); err != nil {
		return errs.Wrap("s.photos.DeleteAll", err)
	}

	return nil
}
