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
	stickers := inmem.NewStickerRepository(users)
	svc := service.NewStickerService(stickers, inmem.NewStickerPackRepository(users, stickers), users, inmem.TxManager{})

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

	// A pack with a price takes votes once, and without them is not taken.
	if err = svc.Add(ctx, ann, "bears"); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Add without votes: %v", err)
	}
	if got := packs(ann)["bears"]; got.Bought || got.Added {
		t.Fatalf("a pack not paid for is taken: %+v", got)
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

func TestStickerService_Studio(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	stickers := inmem.NewStickerRepository(users)
	svc := service.NewStickerService(stickers, inmem.NewStickerPackRepository(users, stickers), users, inmem.TxManager{})

	people := service.NewUserService(users)
	author, err := people.GetOrCreateByEmail(ctx, mustEmail(t, "author@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := people.GetOrCreateByEmail(ctx, mustEmail(t, "buyer@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	view := func(userID uuid.UUID, pack user.StickerPackID) (service.StickerPackView, bool) {
		t.Helper()

		views, err := svc.Catalog(ctx, userID)
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}

		for _, v := range views {
			if v.Pack.ID == pack {
				return v, true
			}
		}

		return service.StickerPackView{}, false
	}

	a, b := uuid.New(), uuid.New()
	draft := user.StickerPackDraft{Name: "Котики", Price: 3}

	p, err := svc.Create(ctx, author, draft, []uuid.UUID{a, b})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Not in the shop yet: its author has it, nobody else sees it.
	if v, ok := view(author, p.ID()); !ok || !v.Added || v.Pack.Author != author {
		t.Fatalf("the author's pack = %+v, %v", v, ok)
	}
	if _, ok := view(buyer, p.ID()); ok {
		t.Fatal("an unpublished pack is in the shop")
	}
	if err = svc.Add(ctx, buyer, p.ID()); !errors.Is(err, user.ErrUnknownStickerPack) {
		t.Fatalf("Add an unpublished pack: %v", err)
	}

	draft.Published = true
	if _, _, err = svc.Edit(ctx, buyer, p.UUID(), service.StickerPackEdit{Draft: draft}); !errors.Is(err, user.ErrCannotChangeStickerPack) {
		t.Fatalf("Edit by another: %v", err)
	}
	if _, _, err = svc.Edit(ctx, author, p.UUID(), service.StickerPackEdit{Draft: draft}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, ok := view(uuid.Nil, p.ID()); !ok {
		t.Fatal("a published pack is not in the shop")
	}

	// The votes paid go to the author; without them nothing is taken.
	if err = svc.Add(ctx, buyer, p.ID()); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Add without votes: %v", err)
	}
	if authored, _ := svc.Authored(ctx, author); authored[0].Holders != 0 {
		t.Fatalf("a pack not paid for has %d holders", authored[0].Holders)
	}
	if _, err = users.Credit(ctx, buyer, 3, 0); err != nil {
		t.Fatal(err)
	}
	if err = svc.Add(ctx, buyer, p.ID()); err != nil {
		t.Fatalf("Add bought: %v", err)
	}
	if u, _ := users.GetByID(ctx, author); u.Wallet().Votes != 3 {
		t.Fatalf("the author has %d votes, want 3", u.Wallet().Votes)
	}

	authors, err := svc.Authors(ctx, []user.Sticker{user.StickerOf(a), "cat_hi", user.StickerOf(uuid.New())})
	if err != nil || len(authors) != 1 || authors[user.StickerOf(a)] != author {
		t.Fatalf("Authors = %v, %v", authors, err)
	}

	authored, err := svc.Authored(ctx, author)
	if err != nil || len(authored) != 1 || authored[0].Holders != 1 {
		t.Fatalf("Authored = %+v, %v", authored, err)
	}

	// Taken by another, it keeps its stickers and stays; it still grows.
	c := uuid.New()
	if _, _, err = svc.Edit(ctx, author, p.UUID(), service.StickerPackEdit{Draft: draft, Removed: []uuid.UUID{a}}); !errors.Is(err, user.ErrStickerPackTaken) {
		t.Fatalf("Remove a sticker of a taken pack: %v", err)
	}
	if _, err = svc.Delete(ctx, author, p.UUID()); !errors.Is(err, user.ErrStickerPackTaken) {
		t.Fatalf("Delete a taken pack: %v", err)
	}
	edited, gone, err := svc.Edit(ctx, author, p.UUID(), service.StickerPackEdit{Draft: draft, Added: []uuid.UUID{c}})
	if err != nil || len(edited.Photos()) != 3 || len(gone) != 0 {
		t.Fatalf("Add a sticker = %v, %v, %v", edited, gone, err)
	}

	// Put out of the shop, it stays with those who took it.
	draft.Published = false
	if _, _, err = svc.Edit(ctx, author, p.UUID(), service.StickerPackEdit{Draft: draft}); err != nil {
		t.Fatal(err)
	}
	if v, ok := view(buyer, p.ID()); !ok || !v.Added {
		t.Fatalf("a bought pack out of the shop = %+v, %v", v, ok)
	}

	// A pack nobody else took is deleted with its photos.
	other, err := svc.Create(ctx, author, user.StickerPackDraft{Name: "Ещё"}, []uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Delete(ctx, buyer, other.UUID()); !errors.Is(err, user.ErrCannotChangeStickerPack) {
		t.Fatalf("Delete by another: %v", err)
	}
	if photos, err := svc.Delete(ctx, author, other.UUID()); err != nil || len(photos) != 1 {
		t.Fatalf("Delete = %v, %v", photos, err)
	}
	if _, ok := view(author, other.ID()); ok {
		t.Fatal("a deleted pack is in the catalogue")
	}
}
