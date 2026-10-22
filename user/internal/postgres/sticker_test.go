package postgres_test

import (
	"context"
	"errors"
	"sync"
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
	svc := service.NewStickerService(stickers, postgres.NewStickerPackRepository(pool), wallets, repo, postgres.NewEconomyRepository(pool), pgpool.NewTxManager(pool))

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

	if _, err = wallets.Credit(ctx, u.ID(), user.StickerPackID("bears").Price(), 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if err = svc.Add(ctx, u.ID(), "bears"); err != nil {
		t.Fatalf("Add bought: %v", err)
	}
	if owned, _ := stickers.List(ctx, u.ID()); len(owned) != 2 {
		t.Fatalf("List after buying = %+v", owned)
	}

	// Purchases of one pack at once are paid for once: the others wait for
	// the votes, find the pack taken and are undone.
	price := user.StickerPackID("cats").Price()
	if _, err = wallets.Credit(ctx, u.ID(), 5*price, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Go(func() { errs[i] = svc.Add(ctx, u.ID(), "cats") })
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("Add at once: %v", err)
		}
	}
	if got, _ := repo.GetByID(ctx, u.ID()); got.Wallet().Votes != 4*price {
		t.Fatalf("votes left %d, want %d", got.Wallet().Votes, 4*price)
	}

	if _, err = stickers.Add(ctx, uuid.New(), "smiles"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Add for nobody: %v", err)
	}
}

func TestStickerPackRepository(t *testing.T) {
	repo, pool := newRepo(t)
	stickers := postgres.NewStickerRepository(pool)
	packs := postgres.NewStickerPackRepository(pool)
	wallets := postgres.NewWalletRepository(pool)
	svc := service.NewStickerService(stickers, packs, wallets, repo, postgres.NewEconomyRepository(pool), pgpool.NewTxManager(pool))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("studio") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	author, buyer := newUser(), newUser()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	draft := user.StickerPackDraft{Name: "Котики", About: "мои", Price: 2, Published: true}

	p, err := svc.Create(ctx, author, draft, []uuid.UUID{b, a})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := packs.Get(ctx, p.UUID())
	if err != nil || got.Draft() != draft || got.AuthorID() != author || len(got.Photos()) != 2 || got.Photos()[0] != b {
		t.Fatalf("Get = %+v %v, %v", got, got.Photos(), err)
	}
	if _, err = packs.Get(ctx, uuid.New()); !errors.Is(err, user.ErrUnknownStickerPack) {
		t.Fatalf("Get nothing: %v", err)
	}

	// Seen by everyone once published; the author has it at once.
	visible, err := packs.Visible(ctx, uuid.Nil)
	if err != nil || !containsPack(visible, p.UUID()) {
		t.Fatalf("Visible = %d packs, %v", len(visible), err)
	}
	if owned, _ := stickers.List(ctx, author); len(owned) != 1 || owned[0].Pack != p.ID() {
		t.Fatalf("the author's stickers = %+v", owned)
	}

	// Bought, the votes go to the author, less the tax of the site: of 2,
	// 1 (90% rounded down).
	if _, err = wallets.Credit(ctx, buyer, 2, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if err = svc.Add(ctx, buyer, p.ID()); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if u, _ := repo.GetByID(ctx, author); u.Wallet().Votes != 1 {
		t.Fatalf("the author has %d votes", u.Wallet().Votes)
	}

	authored, err := packs.ByAuthor(ctx, author)
	if err != nil || len(authored) != 1 || authored[0].Holders != 1 {
		t.Fatalf("ByAuthor = %+v, %v", authored, err)
	}

	// The author is not among those counted.
	if holders, err := packs.Holders(ctx); err != nil || holders[p.ID()] != 1 {
		t.Fatalf("Holders = %d, %v", holders[p.ID()], err)
	}

	if taken, err := packs.TakenByOthers(ctx, p); err != nil || !taken {
		t.Fatalf("TakenByOthers = %v, %v", taken, err)
	}

	edited, _, err := svc.Edit(ctx, author, p.UUID(), service.StickerPackEdit{Draft: user.StickerPackDraft{Name: "Кошки"}, Added: []uuid.UUID{c}})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got, _ := packs.Get(ctx, p.UUID()); got.Draft() != edited.Draft() || len(got.Photos()) != 3 || got.Photos()[2] != c {
		t.Fatalf("after Edit = %+v %v", got.Draft(), got.Photos())
	}

	// Out of the shop, it is the author's and the buyer's only.
	visible, _ = packs.Visible(ctx, newUser())
	if containsPack(visible, p.UUID()) {
		t.Fatal("an unpublished pack is visible")
	}
	visible, _ = packs.Visible(ctx, buyer)
	if !containsPack(visible, p.UUID()) {
		t.Fatal("a bought pack is not visible to its buyer")
	}

	authors, err := packs.Authors(ctx, []uuid.UUID{a, uuid.New()})
	if err != nil || len(authors) != 1 || authors[a] != author {
		t.Fatalf("Authors = %v, %v", authors, err)
	}

	if _, err = svc.Delete(ctx, author, p.UUID()); !errors.Is(err, user.ErrStickerPackTaken) {
		t.Fatalf("Delete a taken pack: %v", err)
	}

	// A pack only its author had goes from their stickers too.
	other, err := svc.Create(ctx, author, user.StickerPackDraft{Name: "Ещё"}, []uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if photos, err := svc.Delete(ctx, author, other.UUID()); err != nil || len(photos) != 1 {
		t.Fatalf("Delete = %v, %v", photos, err)
	}
	if owned, _ := stickers.List(ctx, author); len(owned) != 1 {
		t.Fatalf("the author's stickers after Delete = %+v", owned)
	}

	// An author makes so many packs.
	maker := newUser()
	for range user.MaxStickerPacks {
		if _, err = svc.Create(ctx, maker, draft, []uuid.UUID{uuid.New()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = svc.Create(ctx, maker, draft, []uuid.UUID{uuid.New()}); !errors.Is(err, user.ErrTooManyStickerPacks) {
		t.Fatalf("one pack too many: %v", err)
	}
	if _, err = svc.Create(ctx, uuid.New(), draft, []uuid.UUID{uuid.New()}); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Create for nobody: %v", err)
	}
}

func containsPack(packs []*user.CustomStickerPack, id uuid.UUID) bool {
	for _, p := range packs {
		if p.UUID() == id {
			return true
		}
	}

	return false
}
