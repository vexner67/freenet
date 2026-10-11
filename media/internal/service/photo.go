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
	// Publish keeps the owner's photos for good, those set aside too;
	// missing ones are fine.
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	// Copy keeps the owner's photo from once more, published, as to; a
	// missing from and an existing to are fine.
	Copy(ctx context.Context, ownerID, from, to uuid.UUID) error
	// Delete removes the owner's photos; missing ones are fine.
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	// DeleteAll removes every photo of the owner.
	DeleteAll(ctx context.Context, ownerID uuid.UUID) error
	// DeleteExcept removes the owner's published photos but keep.
	DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error
	// Retire sets aside the photos uploaded before the time and never
	// published, returning how many: the owner has them no more (Load,
	// Existing, Copy), and only Publish brings them back.
	Retire(ctx context.Context, before time.Time) (int, error)
	// DeleteRetired removes the photos set aside that were uploaded
	// before the time, returning how many.
	DeleteRetired(ctx context.Context, before time.Time) (int, error)
}

type PhotoService struct {
	store PhotoStore
	// keepUnpublished is how long a photo may wait for its post, and
	// keepRetired how long after its upload a late Publish still finds it.
	keepUnpublished time.Duration
	keepRetired     time.Duration
	now             func() time.Time
}

func NewPhotoService(store PhotoStore, keepUnpublished, keepRetired time.Duration) *PhotoService {
	return &PhotoService{
		store:           store,
		keepUnpublished: keepUnpublished,
		keepRetired:     keepRetired,
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

// PhotoCopy is a photo to keep once more under a new ID.
type PhotoCopy struct {
	From uuid.UUID
	To   uuid.UUID
}

// Copy keeps photos of the owner once more: those of a post in an album,
// which outlive the post.
func (s *PhotoService) Copy(ctx context.Context, ownerID uuid.UUID, copies []PhotoCopy) error {
	if len(copies) > MaxPhotoIDs {
		return ErrTooManyPhotoIDs
	}

	for _, c := range copies {
		if err := s.store.Copy(ctx, ownerID, c.From, c.To); err != nil {
			return errs.Wrap("s.store.Copy", err)
		}
	}

	return nil
}

// DeleteUnpublished takes away the photos that waited for their post
// longer than they may, returning how many. They are set aside first, not
// deleted: the post may be published already and the word of it late (the
// outbox of its service is stuck), and Publish must still find the photo.
// Those set aside long enough for any word to arrive are deleted.
func (s *PhotoService) DeleteUnpublished(ctx context.Context) (int, error) {
	now := s.now()

	retired, err := s.store.Retire(ctx, now.Add(-s.keepUnpublished))
	if err != nil {
		return retired, errs.Wrap("s.store.Retire", err)
	}

	if _, err = s.store.DeleteRetired(ctx, now.Add(-s.keepRetired)); err != nil {
		return retired, errs.Wrap("s.store.DeleteRetired", err)
	}

	return retired, nil
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
