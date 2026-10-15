package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// PurgeService erases the files of a user whose page was deleted for good.
type PurgeService struct {
	avatars   AvatarStore
	photos    PhotoStore
	documents DocumentStore
}

func NewPurgeService(avatars AvatarStore, photos PhotoStore, documents DocumentStore) *PurgeService {
	return &PurgeService{
		avatars:   avatars,
		photos:    photos,
		documents: documents,
	}
}

// PurgeUser removes all avatars, photos and documents of userID; nothing
// left is fine.
func (s *PurgeService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.avatars.DeleteExcept(ctx, userID, uuid.Nil); err != nil {
		return errs.Wrap("s.avatars.DeleteExcept", err)
	}

	if err := s.photos.DeleteAll(ctx, userID); err != nil {
		return errs.Wrap("s.photos.DeleteAll", err)
	}

	if err := s.documents.DeleteAll(ctx, userID); err != nil {
		return errs.Wrap("s.documents.DeleteAll", err)
	}

	return nil
}
