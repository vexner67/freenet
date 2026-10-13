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
	photos := service.NewPhotoService(inmem.NewPhotoStore(), time.Hour, 24*time.Hour, 0)
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
	photos := service.NewPhotoService(store, time.Hour, 24*time.Hour, 0)
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
	if ok, err := photos.Claim(ctx, ann, []uuid.UUID{fresh, posted}); err != nil || ok {
		t.Fatalf("Claim with a photo set aside = %v, %v", ok, err)
	}
	// None was taken: the fresh upload is still there to post.
	if ok, err := photos.Claim(ctx, ann, []uuid.UUID{fresh}); err != nil || !ok {
		t.Fatalf("Claim of the fresh upload = %v, %v", ok, err)
	}
	if ok, err := photos.Claim(ctx, ann, []uuid.UUID{fresh}); err != nil || ok {
		t.Fatalf("Claim of a claimed photo = %v, %v", ok, err)
	}
	if err := photos.Release(ctx, ann, []uuid.UUID{fresh}); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if ok, err := photos.Claim(ctx, ann, []uuid.UUID{fresh, fresh}); err != nil || ok {
		t.Fatalf("Claim of one photo twice = %v, %v", ok, err)
	}
	if ok, err := photos.Claim(ctx, ann, make([]uuid.UUID, service.MaxPhotoIDs+1)); !errors.Is(err, service.ErrTooManyPhotoIDs) || ok {
		t.Fatalf("Claim of too many = %v, %v", ok, err)
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

// agedClaims is a store whose claims look older than they are.
type agedClaims struct {
	*inmem.PhotoStore
	by time.Duration
}

func (s agedClaims) Claimed(ctx context.Context, ownerID, photoID uuid.UUID) (service.PhotoClaim, bool, error) {
	claim, ok, err := s.PhotoStore.Claimed(ctx, ownerID, photoID)
	claim.At = claim.At.Add(-s.by)

	return claim, ok, err
}

// TestPhotoService_ClaimLeftBehind: a photo claimed for a post nobody
// knows the fate of is its owner's to post again once the claim is old
// and nothing has published the photo.
func TestPhotoService_ClaimLeftBehind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	images := photo.Images{Large: []byte("L"), Small: []byte("S")}

	tests := []struct {
		name string
		keep time.Duration
		aged time.Duration
		want bool
	}{
		{name: "a fresh claim holds", keep: 15 * time.Minute, aged: time.Minute},
		{name: "an old one is taken over", keep: 15 * time.Minute, aged: 16 * time.Minute, want: true},
		{name: "never, when told so", keep: 0, aged: 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := inmem.NewPhotoStore()
			photos := service.NewPhotoService(agedClaims{PhotoStore: store, by: tt.aged}, time.Hour, 24*time.Hour, tt.keep)

			left, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
			for _, id := range []uuid.UUID{left, other} {
				if err := store.Save(ctx, ann, id, images); err != nil {
					t.Fatal(err)
				}
			}

			if ok, err := photos.Claim(ctx, ann, []uuid.UUID{left}); err != nil || !ok {
				t.Fatalf("Claim = %v, %v", ok, err)
			}
			if ok, err := photos.Claim(ctx, ann, []uuid.UUID{other, left}); err != nil || ok != tt.want {
				t.Fatalf("Claim again = %v, %v; want %v", ok, err, tt.want)
			}
			// Refused, it took nothing.
			if ok, err := store.Claim(ctx, ann, other); err != nil || ok == tt.want {
				t.Fatalf("the other photo taken = %v, %v", !ok, err)
			}
		})
	}

	t.Run("a published photo is not", func(t *testing.T) {
		t.Parallel()

		store := inmem.NewPhotoStore()
		photos := service.NewPhotoService(agedClaims{PhotoStore: store, by: time.Hour}, time.Hour, 24*time.Hour, time.Minute)

		id := uuid.Must(uuid.NewV7())
		if err := store.Save(ctx, ann, id, images); err != nil {
			t.Fatal(err)
		}
		if ok, err := photos.Claim(ctx, ann, []uuid.UUID{id}); err != nil || !ok {
			t.Fatalf("Claim = %v, %v", ok, err)
		}
		if err := photos.Publish(ctx, ann, []uuid.UUID{id}); err != nil {
			t.Fatal(err)
		}
		if ok, err := photos.Claim(ctx, ann, []uuid.UUID{id}); err != nil || ok {
			t.Fatalf("Claim of a published photo = %v, %v", ok, err)
		}
	})
}
