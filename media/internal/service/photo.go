package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

// MaxPhotoIDs bounds one Existing or Delete call: the photos of a post.
const MaxPhotoIDs = 10

var ErrTooManyPhotoIDs = errors.New("too many photo ids")

type PhotoStore interface {
	Save(ctx context.Context, ownerID, photoID uuid.UUID, images photo.Images) error
	// Load returns photo.ErrNotFound.
	Load(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error)
	// Existing returns which of photoIDs the owner has.
	Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error)
	// Delete removes the owner's photos; missing ones are fine.
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
}

type PhotoService struct {
	store PhotoStore
}

func NewPhotoService(store PhotoStore) *PhotoService {
	return &PhotoService{
		store: store,
	}
}

// Upload stores a photo of ownerID for a post they are writing.
func (s *PhotoService) Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error) {
	images, err := photo.Make(data)
	if err != nil {
		return uuid.Nil, errs.Wrap("photo.Make", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, errs.Wrap("uuid.NewV7", err)
	}

	if err = s.store.Save(ctx, ownerID, id, images); err != nil {
		return uuid.Nil, errs.Wrap("s.store.Save", err)
	}

	return id, nil
}

func (s *PhotoService) Get(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	data, err := s.store.Load(ctx, ownerID, photoID, size)
	if err != nil {
		return nil, errs.Wrap("s.store.Load", err)
	}

	return data, nil
}

// Existing tells which of the photos are the owner's: a post may only
// show photos its author uploaded.
func (s *PhotoService) Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(photoIDs) > MaxPhotoIDs {
		return nil, ErrTooManyPhotoIDs
	}

	found, err := s.store.Existing(ctx, ownerID, photoIDs)
	if err != nil {
		return nil, errs.Wrap("s.store.Existing", err)
	}

	return found, nil
}

// Delete removes photos of the owner, as when their post is deleted.
func (s *PhotoService) Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if len(photoIDs) > MaxPhotoIDs {
		return ErrTooManyPhotoIDs
	}

	if err := s.store.Delete(ctx, ownerID, photoIDs); err != nil {
		return errs.Wrap("s.store.Delete", err)
	}

	return nil
}
