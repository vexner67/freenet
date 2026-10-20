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
	repo, pool := newRepo(t)
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

	// Views and clicks: one row per reader, of the shown ads only.
	readers := make([]uuid.UUID, 2)
	for i := range readers {
		u, err := user.New(user.Email(unique("adreader") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		readers[i] = u.ID()
	}
	for _, e := range []struct {
		reader uuid.UUID
		event  user.AdEvent
	}{{readers[0], user.AdViewed}, {readers[0], user.AdViewed}, {readers[1], user.AdViewed}, {readers[1], user.AdClicked}} {
		if err = ads.Record(ctx, shown.ID(), e.reader, e.event); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err = ads.Record(ctx, uuid.New(), readers[0], user.AdViewed); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("Record of an unknown ad: %v", err)
	}
	stats, err := ads.Stats(ctx)
	if want := (user.AdStats{Views: 3, Viewers: 2, Clicks: 1, Clickers: 1}); err != nil || stats[shown.ID()] != want {
		t.Fatalf("Stats = %+v, %v", stats[shown.ID()], err)
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
	if stats, _ = ads.Stats(ctx); stats[shown.ID()] != (user.AdStats{}) {
		t.Fatal("the stats outlive their ad")
	}
}
