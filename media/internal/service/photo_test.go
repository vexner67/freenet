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
	photos := service.NewPhotoService(inmem.NewPhotoStore(), time.Hour)
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
