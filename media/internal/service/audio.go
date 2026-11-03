package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/audio"
	"github.com/vexner67/freenet/pkg/errs"
)

type AudioStore interface {
	Save(ctx context.Context, ownerID, audioID uuid.UUID, data []byte) error
	// Load returns audio.ErrNotFound.
	Load(ctx context.Context, ownerID, audioID uuid.UUID) ([]byte, error)
	// Delete removes the owner's songs; missing ones are fine.
	Delete(ctx context.Context, ownerID uuid.UUID, audioIDs []uuid.UUID) error
	// DeleteAll removes every song of the owner.
	DeleteAll(ctx context.Context, ownerID uuid.UUID) error
}

// AudioService keeps the MP3 files of "Аудиозаписи". As a document, a song
// is kept for good as soon as it is uploaded: the post service, which
// knows its artist and title, adds it at once, and the gateway deletes it
// here when that fails.
type AudioService struct {
	store AudioStore
}

func NewAudioService(store AudioStore) *AudioService {
	return &AudioService{store: store}
}

// Upload keeps data as a new song of ownerID and returns its ID.
func (s *AudioService) Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error) {
	if err := audio.Check(data); err != nil {
		return uuid.Nil, errs.Wrap("audio.Check", err)
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

func (s *AudioService) Get(ctx context.Context, ownerID, audioID uuid.UUID) ([]byte, error) {
	data, err := s.store.Load(ctx, ownerID, audioID)
	if err != nil {
		return nil, errs.Wrap("s.store.Load", err)
	}

	return data, nil
}

// Delete removes the owner's songs; missing ones are fine.
func (s *AudioService) Delete(ctx context.Context, ownerID uuid.UUID, audioIDs []uuid.UUID) error {
	return errs.Wrap("s.store.Delete", s.store.Delete(ctx, ownerID, audioIDs))
}
