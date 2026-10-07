package fs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/fs"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/media/internal/service"
)

// Both photo stores must behave the same.
func photoStores(t *testing.T) map[string]service.PhotoStore {
	t.Helper()

	disk, err := fs.NewPhotoStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return map[string]service.PhotoStore{"fs": disk, "inmem": inmem.NewPhotoStore()}
}

func TestPhotoStore(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			first, second := uuid.New(), uuid.New()

			for _, id := range []uuid.UUID{first, second} {
				if err := store.Save(ctx, ann, id, photo.Images{Large: []byte("L-" + id.String()), Small: []byte("S")}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}

			data, err := store.Load(ctx, ann, second, photo.Large)
			if err != nil || string(data) != "L-"+second.String() {
				t.Fatalf("Load large = %q, %v", data, err)
			}
			if data, err = store.Load(ctx, ann, second, photo.Small); err != nil || string(data) != "S" {
				t.Fatalf("Load small = %q, %v", data, err)
			}

			found, err := store.Existing(ctx, ann, []uuid.UUID{first, uuid.New(), second})
			if err != nil || len(found) != 2 {
				t.Fatalf("Existing = %v, %v", found, err)
			}
			if found, _ = store.Existing(ctx, bob, []uuid.UUID{first}); len(found) != 0 {
				t.Fatalf("bob has ann's photo: %v", found)
			}

			if err = store.Delete(ctx, ann, []uuid.UUID{first, uuid.New()}); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, err = store.Load(ctx, ann, first, photo.Large); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("deleted photo: %v", err)
			}
			if _, err = store.Load(ctx, ann, second, photo.Large); err != nil {
				t.Fatalf("the other photo stays: %v", err)
			}

			if _, err = store.Load(ctx, ann, second, "../../etc/passwd"); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("a made-up size must not reach the disk: %v", err)
			}
		})
	}
}

func TestPhotoStore_DeleteAll(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			images := photo.Images{Large: []byte("L"), Small: []byte("S")}
			annsPhoto, bobsPhoto := uuid.New(), uuid.New()

			if err := store.Save(ctx, ann, annsPhoto, images); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, bob, bobsPhoto, images); err != nil {
				t.Fatal(err)
			}

			for range 2 { // missing photos are fine
				if err := store.DeleteAll(ctx, ann); err != nil {
					t.Fatalf("DeleteAll: %v", err)
				}
			}

			if _, err := store.Load(ctx, ann, annsPhoto, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("ann's photo after DeleteAll: %v", err)
			}
			if _, err := store.Load(ctx, bob, bobsPhoto, photo.Small); err != nil {
				t.Fatalf("bob's photo after ann's DeleteAll: %v", err)
			}
		})
	}
}
