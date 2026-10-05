package fs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/media/internal/fs"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/service"
)

// Both stores must behave the same.
func stores(t *testing.T) map[string]service.AvatarStore {
	t.Helper()

	dir := t.TempDir()
	disk, err := fs.NewAvatarStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	return map[string]service.AvatarStore{"fs": disk, "inmem": inmem.NewAvatarStore()}
}

func TestAvatarStore(t *testing.T) {
	t.Parallel()

	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			user, old, current := uuid.New(), uuid.New(), uuid.New()

			for _, id := range []uuid.UUID{old, current} {
				if err := store.Save(ctx, user, id, avatar.Images{Large: []byte("L-" + id.String()), Small: []byte("S")}); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}

			data, err := store.Load(ctx, user, current, avatar.Large)
			if err != nil || string(data) != "L-"+current.String() {
				t.Fatalf("Load large = %q, %v", data, err)
			}
			if data, err = store.Load(ctx, user, current, avatar.Small); err != nil || string(data) != "S" {
				t.Fatalf("Load small = %q, %v", data, err)
			}

			if err = store.DeleteExcept(ctx, user, current); err != nil {
				t.Fatal(err)
			}
			if _, err = store.Load(ctx, user, old, avatar.Large); !errors.Is(err, avatar.ErrNotFound) {
				t.Fatalf("old avatar must be gone: %v", err)
			}
			if _, err = store.Load(ctx, user, current, avatar.Large); err != nil {
				t.Fatalf("kept avatar: %v", err)
			}

			if err = store.DeleteExcept(ctx, user, uuid.Nil); err != nil {
				t.Fatal(err)
			}
			if _, err = store.Load(ctx, user, current, avatar.Large); !errors.Is(err, avatar.ErrNotFound) {
				t.Fatalf("all avatars must be gone: %v", err)
			}
			if err = store.DeleteExcept(ctx, uuid.New(), uuid.Nil); err != nil {
				t.Fatalf("nothing to delete is fine: %v", err)
			}
		})
	}
}

func TestFSStore_NoLeftovers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := fs.NewAvatarStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	user := uuid.New()
	if err = store.Save(context.Background(), user, uuid.New(), avatar.Images{Large: []byte("L"), Small: []byte("S")}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "avatars", user.String()))
	if err != nil || len(entries) != 1 {
		t.Fatalf("one avatar directory and no temporary ones: %v, %v", entries, err)
	}

	if _, err = store.Load(context.Background(), user, uuid.New(), "../../etc/passwd"); !errors.Is(err, avatar.ErrNotFound) {
		t.Fatalf("unknown size must not reach the filesystem: %v", err)
	}
}
