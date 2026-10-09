package fs_test

import (
	"context"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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
			annsPhoto, annsUpload, bobsPhoto := uuid.New(), uuid.New(), uuid.New()

			for _, id := range []uuid.UUID{annsPhoto, annsUpload} {
				if err := store.Save(ctx, ann, id, images); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Publish(ctx, ann, []uuid.UUID{annsPhoto}); err != nil {
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

			if found, err := store.Existing(ctx, ann, []uuid.UUID{annsPhoto, annsUpload}); err != nil || len(found) != 0 {
				t.Fatalf("ann's photos after DeleteAll, published or not: %v, %v", found, err)
			}
			if _, err := store.Load(ctx, bob, bobsPhoto, photo.Small); err != nil {
				t.Fatalf("bob's photo after ann's DeleteAll: %v", err)
			}
		})
	}
}

func TestPhotoStore_Unpublished(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			images := photo.Images{Large: []byte("L"), Small: []byte("S")}
			published, upload, bobsUpload := mustV7(t), mustV7(t), mustV7(t)

			for _, p := range []struct{ owner, id uuid.UUID }{{ann, published}, {ann, upload}, {bob, bobsUpload}} {
				if err := store.Save(ctx, p.owner, p.id, images); err != nil {
					t.Fatal(err)
				}
			}

			for range 2 { // published already and missing photos are fine
				if err := store.Publish(ctx, ann, []uuid.UUID{published, uuid.New()}); err != nil {
					t.Fatalf("Publish: %v", err)
				}
			}
			// bob's name does not publish ann's photo.
			if err := store.Publish(ctx, bob, []uuid.UUID{upload}); err != nil {
				t.Fatal(err)
			}

			if data, err := store.Load(ctx, ann, published, photo.Large); err != nil || string(data) != "L" {
				t.Fatalf("published photo = %q, %v", data, err)
			}
			if found, err := store.Existing(ctx, ann, []uuid.UUID{published, upload}); err != nil || len(found) != 2 {
				t.Fatalf("Existing, published or not = %v, %v", found, err)
			}

			if n, err := store.DeleteUnpublished(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
				t.Fatalf("fresh uploads wait for their post: DeleteUnpublished = %d, %v", n, err)
			}
			if _, err := store.Load(ctx, ann, upload, photo.Small); err != nil {
				t.Fatalf("a fresh upload is shown, as in the post being written: %v", err)
			}

			if n, err := store.DeleteUnpublished(ctx, time.Now().Add(time.Second)); err != nil || n != 2 {
				t.Fatalf("DeleteUnpublished = %d, %v; want ann's and bob's uploads", n, err)
			}
			for _, p := range []struct{ owner, id uuid.UUID }{{ann, upload}, {bob, bobsUpload}} {
				if _, err := store.Load(ctx, p.owner, p.id, photo.Small); !errors.Is(err, photo.ErrNotFound) {
					t.Fatalf("an old upload stays: %v", err)
				}
			}
			if _, err := store.Load(ctx, ann, published, photo.Small); err != nil {
				t.Fatalf("the published photo goes with the uploads: %v", err)
			}

			if err := store.Delete(ctx, ann, []uuid.UUID{published}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(ctx, ann, published, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("deleted published photo: %v", err)
			}
		})
	}
}

// TestPhotoStore_Leftovers: a save that failed half way leaves its
// temporary directory, which goes with the old uploads.
func TestPhotoStore_Leftovers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := fs.NewPhotoStore(root)
	if err != nil {
		t.Fatal(err)
	}

	owner := filepath.Join(root, "uploads", uuid.NewString())
	leftover := filepath.Join(owner, ".upload-123")
	if err = os.MkdirAll(leftover, 0o750); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err = os.Chtimes(leftover, old, old); err != nil {
		t.Fatal(err)
	}

	if n, err := store.DeleteUnpublished(context.Background(), time.Now().Add(-24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("DeleteUnpublished = %d, %v; a leftover is not a photo", n, err)
	}
	if _, err = os.Stat(owner); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("the leftover and the emptied owner's directory must go: %v", err)
	}
}

func mustV7(t *testing.T) uuid.UUID {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func TestPhotoStore_DeleteExcept(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			club := uuid.New()
			images := photo.Images{Large: []byte("L"), Small: []byte("S")}
			left, right, old, upload := uuid.New(), uuid.New(), uuid.New(), uuid.New()

			for _, id := range []uuid.UUID{left, right, old, upload} {
				if err := store.Save(ctx, club, id, images); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Publish(ctx, club, []uuid.UUID{left, right, old}); err != nil {
				t.Fatal(err)
			}

			if err := store.DeleteExcept(ctx, club, []uuid.UUID{left, right}); err != nil {
				t.Fatalf("DeleteExcept: %v", err)
			}
			found, err := store.Existing(ctx, club, []uuid.UUID{left, right, old, upload})
			if err != nil || len(found) != 3 || slices.Contains(found, old) {
				t.Fatalf("after DeleteExcept = %v, %v; the kept two and the upload", found, err)
			}

			if err = store.DeleteExcept(ctx, uuid.New(), nil); err != nil {
				t.Fatalf("an owner without photos: %v", err)
			}
		})
	}
}
