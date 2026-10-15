package fs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/media/internal/fs"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/s3"
	"github.com/vexner67/freenet/media/internal/service"
)

// All stores must behave the same.
func documentStores(t *testing.T) map[string]service.DocumentStore {
	t.Helper()

	disk, err := fs.NewDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	all := map[string]service.DocumentStore{"fs": disk, "inmem": inmem.NewDocumentStore()}
	if b := bucket(t); b != nil {
		all["s3"] = s3.NewDocumentStore(b)
	}

	return all
}

func TestDocumentStore(t *testing.T) {
	t.Parallel()

	for name, store := range documentStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ann, bob := uuid.New(), uuid.New()
			first, second, bobs := uuid.New(), uuid.New(), uuid.New()

			for _, d := range []struct{ owner, id uuid.UUID }{{ann, first}, {ann, second}, {bob, bobs}} {
				if err := store.Save(ctx, d.owner, d.id, []byte("doc "+d.id.String())); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}

			data, err := store.Load(ctx, ann, first)
			if err != nil || string(data) != "doc "+first.String() {
				t.Fatalf("Load = %q, %v", data, err)
			}
			if _, err = store.Load(ctx, bob, first); !errors.Is(err, document.ErrNotFound) {
				t.Fatalf("under another owner: %v", err)
			}

			if err = store.Delete(ctx, ann, []uuid.UUID{first, uuid.New()}); err != nil {
				t.Fatalf("Delete with a missing one: %v", err)
			}
			if _, err = store.Load(ctx, ann, first); !errors.Is(err, document.ErrNotFound) {
				t.Fatalf("a deleted document: %v", err)
			}
			if _, err = store.Load(ctx, ann, second); err != nil {
				t.Fatalf("the other document: %v", err)
			}

			if err = store.DeleteAll(ctx, ann); err != nil {
				t.Fatal(err)
			}
			if _, err = store.Load(ctx, ann, second); !errors.Is(err, document.ErrNotFound) {
				t.Fatalf("after DeleteAll: %v", err)
			}
			if _, err = store.Load(ctx, bob, bobs); err != nil {
				t.Fatalf("bob's document: %v", err)
			}
			if err = store.DeleteAll(ctx, uuid.New()); err != nil {
				t.Fatalf("nothing to delete is fine: %v", err)
			}
		})
	}
}
