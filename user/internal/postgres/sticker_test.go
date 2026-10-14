package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestStickerRepository(t *testing.T) {
	repo, pool := newRepo(t)
	stickers := postgres.NewStickerRepository(pool)
	wallets := postgres.NewWalletRepository(pool)
	svc := service.NewStickerService(stickers, wallets, pgpool.NewTxManager(pool))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := user.New(user.Email(unique("sticker") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	if taken, err := stickers.Add(ctx, u.ID(), "smiles"); err != nil || !taken {
		t.Fatalf("Add = %v, %v", taken, err)
	}
	if taken, err := stickers.Add(ctx, u.ID(), "smiles"); err != nil || taken {
		t.Fatalf("Add again = %v, %v", taken, err)
	}
	if err = stickers.Remove(ctx, u.ID(), "smiles"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err = stickers.Remove(ctx, u.ID(), "bears"); err != nil {
		t.Fatalf("Remove a pack never taken: %v", err)
	}
	if owned, err := stickers.List(ctx, u.ID()); err != nil || len(owned) != 1 || owned[0] != (user.OwnedPack{Pack: "smiles"}) {
		t.Fatalf("List = %+v, %v", owned, err)
	}

	// Without the votes, the pack is not taken: the transaction is undone.
	if err = svc.Add(ctx, u.ID(), "bears"); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Add without votes: %v", err)
	}
	if owned, _ := stickers.List(ctx, u.ID()); len(owned) != 1 {
		t.Fatalf("a pack not paid for is taken: %+v", owned)
	}

	if _, err = wallets.Credit(ctx, u.ID(), user.StickerPackID("bears").Price(), 0); err != nil {
		t.Fatal(err)
	}
	if err = svc.Add(ctx, u.ID(), "bears"); err != nil {
		t.Fatalf("Add bought: %v", err)
	}
	if owned, _ := stickers.List(ctx, u.ID()); len(owned) != 2 {
		t.Fatalf("List after buying = %+v", owned)
	}

	if _, err = stickers.Add(ctx, uuid.New(), "smiles"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Add for nobody: %v", err)
	}
}
