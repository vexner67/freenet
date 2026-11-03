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
	audio     AudioStore
}

func NewPurgeService(avatars AvatarStore, photos PhotoStore, documents DocumentStore, audio AudioStore) *PurgeService {
	return &PurgeService{
		avatars:   avatars,
		photos:    photos,
		documents: documents,
		audio:     audio,
	}
}

// PurgeUser removes all avatars, photos, documents and songs of userID; nothing
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

	if err := s.audio.DeleteAll(ctx, userID); err != nil {
		return errs.Wrap("s.audio.DeleteAll", err)
	}

	return nil
}
