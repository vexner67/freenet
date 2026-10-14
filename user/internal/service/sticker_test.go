package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestStickerService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	svc := service.NewStickerService(inmem.NewStickerRepository(users), users, inmem.TxManager{})

	ann, err := service.NewUserService(users).GetOrCreateByEmail(ctx, mustEmail(t, "ann@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	packs := func(userID uuid.UUID) map[user.StickerPackID]service.StickerPackView {
		t.Helper()

		views, err := svc.Catalog(ctx, userID)
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}

		out := map[user.StickerPackID]service.StickerPackView{}
		for _, v := range views {
			out[v.Pack.ID] = v
		}

		return out
	}

	if got := packs(uuid.Nil); len(got) != len(user.StickerCatalog()) || got["smiles"].Bought || got["smiles"].Added {
		t.Fatalf("the catalogue of a guest = %+v", got)
	}

	// A free pack costs nothing.
	if err = svc.Add(ctx, ann, "smiles"); err != nil {
		t.Fatalf("Add a free pack: %v", err)
	}
	if got := packs(ann)["smiles"]; !got.Bought || !got.Added {
		t.Fatalf("a free pack taken = %+v", got)
	}

	// A pack with a price takes votes once. That the pack is not taken
	// without them is the transaction's: see the Postgres test.
	if err = svc.Add(ctx, ann, "bears"); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Add without votes: %v", err)
	}

	price := user.StickerPackID("cats").Price()
	if _, err = users.Credit(ctx, ann, price, 0); err != nil {
		t.Fatal(err)
	}
	if err = svc.Add(ctx, ann, "cats"); err != nil {
		t.Fatalf("Add a pack bought: %v", err)
	}
	if u, _ := users.GetByID(ctx, ann); u.Wallet().Votes != 0 {
		t.Fatalf("votes left %d, want 0", u.Wallet().Votes)
	}

	// Put away, it stays bought and comes back free.
	if err = svc.Remove(ctx, ann, "cats"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := packs(ann)["cats"]; !got.Bought || got.Added {
		t.Fatalf("a pack put away = %+v", got)
	}
	if err = svc.Add(ctx, ann, "cats"); err != nil {
		t.Fatalf("Add back: %v", err)
	}
	if got := packs(ann)["cats"]; !got.Added {
		t.Fatalf("a pack added back = %+v", got)
	}

	if err = svc.Add(ctx, uuid.New(), "smiles"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Add for nobody: %v", err)
	}
}
