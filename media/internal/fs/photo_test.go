package fs_test

import (
	"context"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

			if _, err = store.Load(ctx, bob, first, photo.Large); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("bob has ann's photo: %v", err)
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

// TestPhotoStore_Claim: an upload goes to one post, the first to ask.
func TestPhotoStore_Claim(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			images := photo.Images{Large: []byte("L"), Small: []byte("S")}
			upload, posted := mustV7(t), mustV7(t)

			for _, id := range []uuid.UUID{upload, posted} {
				if err := store.Save(ctx, ann, id, images); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Publish(ctx, ann, []uuid.UUID{posted}); err != nil {
				t.Fatal(err)
			}

			claim := func(owner, id uuid.UUID) bool {
				t.Helper()

				taken, err := store.Claim(ctx, owner, id)
				if err != nil {
					t.Fatalf("Claim: %v", err)
				}

				return taken
			}

			if claim(bob, upload) {
				t.Fatal("bob claimed ann's upload")
			}
			if claim(ann, uuid.New()) {
				t.Fatal("a photo that is not there was claimed")
			}
			if claim(ann, posted) {
				t.Fatal("a published photo, which a post shows already, was claimed")
			}
			if !claim(ann, upload) {
				t.Fatal("ann's upload is hers to post")
			}
			if claim(ann, upload) {
				t.Fatal("a photo was claimed twice")
			}
			if _, err := store.Load(ctx, ann, upload, photo.Small); err != nil {
				t.Fatalf("a claimed photo is shown: %v", err)
			}

			// The post was refused: the photo waits for the next attempt.
			for range 2 { // released already is fine
				if err := store.Release(ctx, ann, upload); err != nil {
					t.Fatalf("Release: %v", err)
				}
			}
			if err := store.Release(ctx, ann, uuid.New()); err != nil {
				t.Fatalf("Release of a missing photo: %v", err)
			}
			if !claim(ann, upload) {
				t.Fatal("a released photo is claimed again")
			}

			// Published, it is claimed for good, released or not.
			if err := store.Publish(ctx, ann, []uuid.UUID{upload}); err != nil {
				t.Fatal(err)
			}
			if err := store.Release(ctx, ann, upload); err != nil {
				t.Fatal(err)
			}
			if claim(ann, upload) {
				t.Fatal("a published photo was claimed after a release")
			}
			if data, err := store.Load(ctx, ann, upload, photo.Large); err != nil || string(data) != "L" {
				t.Fatalf("the published photo = %q, %v", data, err)
			}
		})
	}
}

// TestPhotoStore_ClaimOnce: of claims made at once one wins.
func TestPhotoStore_ClaimOnce(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, upload := uuid.New(), mustV7(t)
			if err := store.Save(ctx, ann, upload, photo.Images{Large: []byte("L"), Small: []byte("S")}); err != nil {
				t.Fatal(err)
			}

			var (
				wg    sync.WaitGroup
				taken atomic.Int32
			)
			for range 8 {
				wg.Go(func() {
					if ok, err := store.Claim(ctx, ann, upload); err != nil {
						t.Errorf("Claim: %v", err)
					} else if ok {
						taken.Add(1)
					}
				})
			}
			wg.Wait()

			if n := taken.Load(); n != 1 {
				t.Fatalf("%d claims took the photo, want one", n)
			}
		})
	}
}

func TestPhotoStore_Copy(t *testing.T) {
	t.Parallel()

	for name, store := range photoStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			posted, upload, inAlbum, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()

			for _, id := range []uuid.UUID{posted, upload} {
				if err := store.Save(ctx, ann, id, photo.Images{Large: []byte("L-" + id.String()), Small: []byte("S")}); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Publish(ctx, ann, []uuid.UUID{posted}); err != nil {
				t.Fatal(err)
			}

			for range 2 { // a copy made already is fine
				if err := store.Copy(ctx, ann, posted, inAlbum); err != nil {
					t.Fatalf("Copy: %v", err)
				}
			}
			// The relay may copy before it publishes; a missing photo and
			// one of another owner are fine and copy nothing.
			if err := store.Copy(ctx, ann, upload, second); err != nil {
				t.Fatalf("Copy of an upload: %v", err)
			}
			if err := store.Copy(ctx, ann, uuid.New(), uuid.New()); err != nil {
				t.Fatalf("Copy of a missing photo: %v", err)
			}
			stolen := uuid.New()
			if err := store.Copy(ctx, bob, posted, stolen); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(ctx, bob, stolen, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("bob copied ann's photo: %v", err)
			}

			// The copies outlive the post and the cleanup of uploads.
			if err := store.Delete(ctx, ann, []uuid.UUID{posted, upload}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Retire(ctx, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.DeleteRetired(ctx, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}

			for from, to := range map[uuid.UUID]uuid.UUID{posted: inAlbum, upload: second} {
				if data, err := store.Load(ctx, ann, to, photo.Large); err != nil || string(data) != "L-"+from.String() {
					t.Fatalf("the copy = %q, %v", data, err)
				}
				if data, err := store.Load(ctx, ann, to, photo.Small); err != nil || string(data) != "S" {
					t.Fatalf("the small copy = %q, %v", data, err)
				}
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

			for _, id := range []uuid.UUID{annsPhoto, annsUpload} {
				if _, err := store.Load(ctx, ann, id, photo.Small); !errors.Is(err, photo.ErrNotFound) {
					t.Fatalf("ann's photo after DeleteAll, published or not: %v", err)
				}
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
			if _, err := store.Load(ctx, ann, upload, photo.Large); err != nil {
				t.Fatalf("bob published ann's upload away: %v", err)
			}

			if n, err := store.Retire(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
				t.Fatalf("fresh uploads wait for their post: Retire = %d, %v", n, err)
			}
			if _, err := store.Load(ctx, ann, upload, photo.Small); err != nil {
				t.Fatalf("a fresh upload is shown, as in the post being written: %v", err)
			}

			late := mustV7(t)
			if err := store.Save(ctx, ann, late, images); err != nil {
				t.Fatal(err)
			}

			if n, err := store.Retire(ctx, time.Now().Add(time.Second)); err != nil || n != 3 {
				t.Fatalf("Retire = %d, %v; want ann's two and bob's uploads", n, err)
			}
			if n, err := store.Retire(ctx, time.Now().Add(time.Second)); err != nil || n != 0 {
				t.Fatalf("Retire again = %d, %v; nothing is left to set aside", n, err)
			}
			for _, p := range []struct{ owner, id uuid.UUID }{{ann, upload}, {ann, late}, {bob, bobsUpload}} {
				if _, err := store.Load(ctx, p.owner, p.id, photo.Small); !errors.Is(err, photo.ErrNotFound) {
					t.Fatalf("an old upload stays: %v", err)
				}
				if taken, err := store.Claim(ctx, p.owner, p.id); err != nil || taken {
					t.Fatalf("an old upload is still its owner's to post: %v, %v", taken, err)
				}
			}
			if _, err := store.Load(ctx, ann, published, photo.Small); err != nil {
				t.Fatalf("the published photo goes with the uploads: %v", err)
			}

			// A photo set aside is not copied: its owner has it no more.
			copied := mustV7(t)
			if err := store.Copy(ctx, ann, late, copied); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(ctx, ann, copied, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("a copy of a photo set aside: %v", err)
			}

			// The word of a post published long ago arrives late: its photo
			// comes back, and stays whatever is deleted afterwards.
			if n, err := store.DeleteRetired(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
				t.Fatalf("DeleteRetired of the fresh = %d, %v", n, err)
			}
			if err := store.Publish(ctx, ann, []uuid.UUID{late}); err != nil {
				t.Fatalf("late Publish: %v", err)
			}
			if data, err := store.Load(ctx, ann, late, photo.Large); err != nil || string(data) != "L" {
				t.Fatalf("a photo published late = %q, %v", data, err)
			}

			// A deleted post takes its photo set aside with it.
			if err := store.Delete(ctx, bob, []uuid.UUID{bobsUpload}); err != nil {
				t.Fatal(err)
			}
			if n, err := store.DeleteRetired(ctx, time.Now().Add(time.Second)); err != nil || n != 1 {
				t.Fatalf("DeleteRetired = %d, %v; want ann's upload alone", n, err)
			}
			if err := store.Publish(ctx, ann, []uuid.UUID{upload}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(ctx, ann, upload, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("a photo deleted for good came back: %v", err)
			}
			if _, err := store.Load(ctx, ann, late, photo.Small); err != nil {
				t.Fatalf("the photo published late went with those set aside: %v", err)
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

	if n, err := store.Retire(context.Background(), time.Now().Add(-24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("Retire = %d, %v; a leftover is not a photo", n, err)
	}
	if _, err = os.Stat(owner); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("the leftover and the emptied owner's directory must go: %v", err)
	}
	if left, err := os.ReadDir(filepath.Join(root, "retired")); err != nil || len(left) != 0 {
		t.Fatalf("a leftover is deleted, not set aside: %v, %v", left, err)
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
			for _, id := range []uuid.UUID{left, right, upload} {
				if _, err := store.Load(ctx, club, id, photo.Small); err != nil {
					t.Fatalf("the kept two and the upload stay: %v", err)
				}
			}
			if _, err := store.Load(ctx, club, old, photo.Small); !errors.Is(err, photo.ErrNotFound) {
				t.Fatalf("the photo not kept: %v", err)
			}

			if err := store.DeleteExcept(ctx, uuid.New(), nil); err != nil {
				t.Fatalf("an owner without photos: %v", err)
			}
		})
	}
}
