package service_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/media/internal/service"
)

func TestPhotoService_Unpublished(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	photos := service.NewPhotoService(inmem.NewPhotoStore(), time.Hour, 24*time.Hour)
	ann := uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 60, 40))); err != nil {
		t.Fatal(err)
	}

	id, err := photos.Upload(ctx, ann, picture.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// Within the hour the upload waits for its post.
	if n, err := photos.DeleteUnpublished(ctx); err != nil || n != 0 {
		t.Fatalf("DeleteUnpublished = %d, %v", n, err)
	}
	if _, err = photos.Get(ctx, ann, id, photo.Small); err != nil {
		t.Fatalf("fresh upload: %v", err)
	}

	if err = photos.Publish(ctx, ann, []uuid.UUID{id}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err = photos.Publish(ctx, ann, make([]uuid.UUID, service.MaxPhotoIDs+1)); !errors.Is(err, service.ErrTooManyPhotoIDs) {
		t.Fatalf("Publish of too many: %v", err)
	}
}

// TestPhotoService_PublishedLate: a photo that waited too long is gone for
// its owner, yet the word that its post was published, arriving late (the
// outbox of post was stuck), still brings it back.
func TestPhotoService_PublishedLate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewPhotoStore()
	photos := service.NewPhotoService(store, time.Hour, 24*time.Hour)
	ann := uuid.New()
	images := photo.Images{Large: []byte("L"), Small: []byte("S")}

	posted, forgotten, fresh := uploadedAgo(2*time.Hour), uploadedAgo(25*time.Hour), uploadedAgo(time.Minute)
	for _, id := range []uuid.UUID{posted, forgotten, fresh} {
		if err := store.Save(ctx, ann, id, images); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := photos.DeleteUnpublished(ctx); err != nil || n != 2 {
		t.Fatalf("DeleteUnpublished = %d, %v; want the two older than an hour", n, err)
	}
	if own, err := photos.Existing(ctx, ann, []uuid.UUID{posted, forgotten, fresh}); err != nil || len(own) != 1 || own[0] != fresh {
		t.Fatalf("Existing = %v, %v; want the fresh upload alone", own, err)
	}
	if _, err := photos.Get(ctx, ann, posted, photo.Small); !errors.Is(err, photo.ErrNotFound) {
		t.Fatalf("a photo set aside is shown: %v", err)
	}

	if err := photos.Publish(ctx, ann, []uuid.UUID{posted, forgotten}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := photos.Get(ctx, ann, posted, photo.Small); err != nil {
		t.Fatalf("a photo published late: %v", err)
	}
	// Older than a day: deleted for good by the same cleanup.
	if _, err := photos.Get(ctx, ann, forgotten, photo.Small); !errors.Is(err, photo.ErrNotFound) {
		t.Fatalf("a photo deleted for good: %v", err)
	}

	if n, err := photos.DeleteUnpublished(ctx); err != nil || n != 0 {
		t.Fatalf("DeleteUnpublished again = %d, %v", n, err)
	}
	if _, err := photos.Get(ctx, ann, posted, photo.Small); err != nil {
		t.Fatalf("the published photo after a cleanup: %v", err)
	}
}

// uploadedAgo is the ID of a photo uploaded that long ago: a UUIDv7 keeps
// its time in the first 48 bits, in milliseconds.
func uploadedAgo(ago time.Duration) uuid.UUID {
	id := uuid.Must(uuid.NewV7())

	ms := time.Now().Add(-ago).UnixMilli()
	for i := range 6 {
		id[i] = byte(ms >> (8 * (5 - i)))
	}

	return id
}
