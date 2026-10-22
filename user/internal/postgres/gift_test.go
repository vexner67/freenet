package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestGiftRepository(t *testing.T) {
	repo, pool := newRepo(t)
	gifts := postgres.NewGiftRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("gift") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	send := func(to *user.User, from uuid.UUID, kind user.GiftKind, caption user.GiftCaption, anonymous bool) *user.Gift {
		t.Helper()

		g, err := to.GiftFrom(from, user.Ties{}, kind, caption, anonymous)
		if err != nil {
			t.Fatal(err)
		}
		if err = gifts.Save(ctx, g); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return g
	}

	ann, bob, cat := newUser(), newUser(), newUser()

	first := send(ann, bob.ID(), "freenet_coin", "Привет", false)
	second := send(ann, cat.ID(), "freenet_ring", "", true)
	third := send(ann, bob.ID(), "freenet_crystal", "Ещё", false)
	send(bob, ann.ID(), "freenet_heart", "", false)

	got, err := gifts.Get(ctx, second.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.RecipientID() != ann.ID() || got.SenderID() != cat.ID() || got.Kind() != "freenet_ring" || got.Caption() != "" || !got.Anonymous() {
		t.Fatalf("Get = %+v", got)
	}

	if _, err = gifts.Get(ctx, uuid.New()); !errors.Is(err, user.ErrGiftNotFound) {
		t.Fatalf("Get of nothing: %v", err)
	}

	ids := func(list []*user.Gift) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(list))
		for _, g := range list {
			out = append(out, g.ID())
		}
		return out
	}

	// Newest first, keyset by ID.
	page, err := gifts.List(ctx, ann.ID(), uuid.Nil, 2)
	if err != nil || len(page) != 2 || page[0].ID() != third.ID() || page[1].ID() != second.ID() {
		t.Fatalf("List = %v, %v", ids(page), err)
	}
	if page[0].Caption() != "Ещё" || page[0].SenderID() != bob.ID() {
		t.Fatalf("the newest gift = %+v", page[0])
	}
	if page, err = gifts.List(ctx, ann.ID(), second.ID(), 2); err != nil || len(page) != 1 || page[0].ID() != first.ID() {
		t.Fatalf("the second page = %v, %v", ids(page), err)
	}
	if count, err := gifts.Count(ctx, ann.ID()); err != nil || count != 3 {
		t.Fatalf("Count = %d, %v", count, err)
	}

	// Delete is idempotent.
	for range 2 {
		if err = gifts.Delete(ctx, first.ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	}
	if count, _ := gifts.Count(ctx, ann.ID()); count != 2 {
		t.Fatalf("Count after Delete = %d", count)
	}

	// A gift to or from nobody is refused.
	ghost, err := user.New(user.Email(unique("ghost") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := ghost.GiftFrom(bob.ID(), user.Ties{}, "freenet_coin", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.Save(ctx, orphan); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Save to nobody: %v", err)
	}
	if orphan, err = ann.GiftFrom(ghost.ID(), user.Ties{}, "freenet_coin", "", false); err != nil {
		t.Fatal(err)
	}
	if err = gifts.Save(ctx, orphan); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Save from nobody: %v", err)
	}

	// The erased sender leaves the gift unsigned; the erased recipient
	// takes the gifts along.
	if _, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, cat.ID()); err != nil {
		t.Fatal(err)
	}
	if got, err = gifts.Get(ctx, second.ID()); err != nil || got.SenderID() != uuid.Nil || !got.Anonymous() {
		t.Fatalf("after the sender is erased: %+v, %v", got, err)
	}

	if _, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ann.ID()); err != nil {
		t.Fatal(err)
	}
	if count, err := gifts.Count(ctx, ann.ID()); err != nil || count != 0 {
		t.Fatalf("after the recipient is erased: %d, %v", count, err)
	}
	if got, err = gifts.Get(ctx, second.ID()); !errors.Is(err, user.ErrGiftNotFound) {
		t.Fatalf("a gift of an erased page: %+v, %v", got, err)
	}
	// What ann sent to bob stays, unsigned.
	if page, err = gifts.List(ctx, bob.ID(), uuid.Nil, 10); err != nil || len(page) != 1 || page[0].SenderID() != uuid.Nil {
		t.Fatalf("bob's gifts: %v, %v", ids(page), err)
	}
}

func TestGiftRepository_Prices(t *testing.T) {
	_, pool := newRepo(t)
	gifts := postgres.NewGiftRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, price := range []user.GiftPrice{9, 0} {
		if err := gifts.SetPrice(ctx, "freenet_cup", price); err != nil {
			t.Fatal(err)
		}

		prices, err := gifts.Prices(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := prices["freenet_cup"]; !ok || got != price {
			t.Fatalf("the price set to %d reads %d, %t", price, got, ok)
		}
	}
}
