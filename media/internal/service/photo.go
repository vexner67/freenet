package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

// MaxPhotoIDs bounds one Existing or Delete call: the photos of a post.
const MaxPhotoIDs = 10

var ErrTooManyPhotoIDs = errors.New("too many photo ids")

// PhotoStore keeps photos; a photo is saved unpublished. Photo IDs are
// UUIDv7: the store tells the upload time by them.
type PhotoStore interface {
	Save(ctx context.Context, ownerID, photoID uuid.UUID, images photo.Images) error
	// Load returns photo.ErrNotFound.
	Load(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error)
	// Existing returns which of photoIDs the owner has, published or not.
	Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error)
	// Publish keeps the owner's photos for good; missing ones are fine.
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	// Delete removes the owner's photos; missing ones are fine.
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	// DeleteAll removes every photo of the owner.
	DeleteAll(ctx context.Context, ownerID uuid.UUID) error
	// DeleteExcept removes the owner's published photos but keep.
	DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error
	// DeleteUnpublished removes the photos uploaded before the time and
	// never published, returning how many.
	DeleteUnpublished(ctx context.Context, before time.Time) (int, error)
}

type PhotoService struct {
	store PhotoStore
	// keepUnpublished is how long a photo may wait for its post.
	keepUnpublished time.Duration
	now             func() time.Time
}

func NewPhotoService(store PhotoStore, keepUnpublished time.Duration) *PhotoService {
	return &PhotoService{
		store:           store,
		keepUnpublished: keepUnpublished,
		now:             time.Now,
	}
}

// Upload stores a photo of ownerID for a post they are writing. Until the
// post is published (Publish), the photo is kept only for a while: one
// attached to a post that was never sent is not left forever.
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

// Publish keeps the photos of a published post for good.
func (s *PhotoService) Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if len(photoIDs) > MaxPhotoIDs {
		return ErrTooManyPhotoIDs
	}

	if err := s.store.Publish(ctx, ownerID, photoIDs); err != nil {
		return errs.Wrap("s.store.Publish", err)
	}

	return nil
}

// DeleteUnpublished deletes the photos that waited for their post longer
// than they may, returning how many.
func (s *PhotoService) DeleteUnpublished(ctx context.Context) (int, error) {
	deleted, err := s.store.DeleteUnpublished(ctx, s.now().Add(-s.keepUnpublished))
	if err != nil {
		return deleted, errs.Wrap("s.store.DeleteUnpublished", err)
	}

	return deleted, nil
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

// UploadKept stores a photo of ownerID, a community, for good at once:
// its backdrop ("Фон страницы"), which no post publishes.
func (s *PhotoService) UploadKept(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error) {
	id, err := s.Upload(ctx, ownerID, data)
	if err != nil {
		return uuid.Nil, errs.Wrap("s.Upload", err)
	}

	if err = s.store.Publish(ctx, ownerID, []uuid.UUID{id}); err != nil {
		return uuid.Nil, errs.Wrap("s.store.Publish", err)
	}

	return id, nil
}

// DeleteExcept removes the kept photos of ownerID but keep, those it no
// longer shows.
func (s *PhotoService) DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error {
	if len(keep) > MaxPhotoIDs {
		return ErrTooManyPhotoIDs
	}

	return errs.Wrap("s.store.DeleteExcept", s.store.DeleteExcept(ctx, ownerID, keep))
}
