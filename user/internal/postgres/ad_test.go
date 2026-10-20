package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestAdRepository(t *testing.T) {
	_, pool := newRepo(t)
	ads := postgres.NewAdRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cover := user.AdPhoto{OwnerID: uuid.New(), PhotoID: uuid.New()}
	shown, err := user.NewAd(user.AdDraft{URL: "https://a.example", Title: "A", Text: "Скидки\nна всё", Photo: cover, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	hidden, _ := user.NewAd(user.AdDraft{URL: "https://b.example", Title: "B"})

	for _, ad := range []*user.Ad{shown, hidden} {
		if err = ads.Save(ctx, ad); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = ads.Delete(context.Background(), ad.ID()) })
	}

	has := func(all bool, id uuid.UUID) *user.Ad {
		list, err := ads.List(ctx, all)
		if err != nil {
			t.Fatal(err)
		}
		for _, ad := range list {
			if ad.ID() == id {
				return ad
			}
		}
		return nil
	}

	if got := has(false, shown.ID()); got == nil || got.Photo() != cover || got.Text() != "Скидки\nна всё" || got.URL() != "https://a.example" {
		t.Fatalf("the shown ad: %+v", got)
	}
	if has(false, hidden.ID()) != nil || has(true, hidden.ID()) == nil {
		t.Fatal("the hidden ad is listed wrong")
	}

	on := true
	none := user.AdPhoto{}
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		ad, err := ads.Lock(ctx, hidden.ID())
		if err != nil {
			return err
		}
		edited, _, err := ad.Edited(user.AdChange{Active: &on, Photo: &none})
		if err != nil {
			return err
		}
		return ads.Update(ctx, edited)
	})
	if err != nil || has(false, hidden.ID()) == nil {
		t.Fatalf("showing the hidden ad: %v", err)
	}

	if _, err = ads.Lock(ctx, uuid.New()); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("Lock of an unknown ad: %v", err)
	}

	deleted, err := ads.Delete(ctx, shown.ID())
	if err != nil || deleted.Photo() != cover {
		t.Fatalf("Delete = %+v, %v", deleted, err)
	}
	if _, err = ads.Delete(ctx, shown.ID()); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}
