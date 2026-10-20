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

func TestAdService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	repo := inmem.NewAdRepository()
	ads := service.NewAdService(w.users, repo, repo, inmem.TxManager{}, admins)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	cover := uuid.New()

	draft := user.AdDraft{URL: "https://a.example", Title: "A", Photo: user.AdPhoto{PhotoID: cover}, Active: true}
	if _, err := ads.Create(ctx, ann, draft); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Create by ann: %v", err)
	}

	shown, err := ads.Create(ctx, admin, draft)
	if err != nil || shown.Photo() != (user.AdPhoto{OwnerID: admin, PhotoID: cover}) {
		t.Fatalf("Create = %+v, %v", shown, err)
	}

	hiddenDraft := user.AdDraft{URL: "https://b.example", Title: "B"}
	hidden, err := ads.Create(ctx, admin, hiddenDraft)
	if err != nil {
		t.Fatal(err)
	}

	// Everyone reads the shown ads; the hidden only the administrators.
	if got, err := ads.Shown(ctx); err != nil || len(got) != 1 || got[0].ID() != shown.ID() {
		t.Fatalf("Shown = %v, %v", got, err)
	}
	if _, _, err = ads.All(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("All by ann: %v", err)
	}
	if got, _, err := ads.All(ctx, admin); err != nil || len(got) != 2 || got[0].ID() != hidden.ID() {
		t.Fatalf("All = %v, %v", got, err)
	}

	// The readers' views and clicks are counted, of the shown ads only.
	for _, e := range []struct {
		reader uuid.UUID
		event  user.AdEvent
	}{{ann, user.AdViewed}, {ann, user.AdViewed}, {admin, user.AdViewed}, {ann, user.AdClicked}} {
		if err = ads.Record(ctx, e.reader, shown.ID(), e.event); err != nil {
			t.Fatalf("Record %s: %v", e.event, err)
		}
	}
	if err = ads.Record(ctx, ann, hidden.ID(), user.AdViewed); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("a hidden ad counted: %v", err)
	}
	_, stats, err := ads.All(ctx, admin)
	if want := (user.AdStats{Views: 3, Viewers: 2, Clicks: 1, Clickers: 1}); err != nil || stats[shown.ID()] != want {
		t.Fatalf("stats = %+v, %v", stats[shown.ID()], err)
	}
	if _, ok := stats[hidden.ID()]; ok {
		t.Fatal("the hidden ad has stats")
	}

	// Showing the hidden one and changing its cover.
	on := true
	newCover := user.AdPhoto{PhotoID: uuid.New()}
	if _, _, err = ads.Edit(ctx, ann, hidden.ID(), user.AdChange{Active: &on}); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Edit by ann: %v", err)
	}
	if _, _, err = ads.Edit(ctx, admin, uuid.New(), user.AdChange{Active: &on}); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("an unknown ad: %v", err)
	}
	edited, dropped, err := ads.Edit(ctx, admin, hidden.ID(), user.AdChange{Active: &on, Photo: &newCover})
	if err != nil || !edited.IsActive() || edited.Photo().OwnerID != admin || !dropped.IsZero() {
		t.Fatalf("Edit = %+v, %+v, %v", edited, dropped, err)
	}
	if got, _ := ads.Shown(ctx); len(got) != 2 {
		t.Fatalf("Shown after showing: %d", len(got))
	}

	// Deleting gives the cover to drop.
	if _, err = ads.Delete(ctx, ann, shown.ID()); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Delete by ann: %v", err)
	}
	if photo, err := ads.Delete(ctx, admin, shown.ID()); err != nil || photo.PhotoID != cover {
		t.Fatalf("Delete = %+v, %v", photo, err)
	}
	if _, err = ads.Delete(ctx, admin, shown.ID()); !errors.Is(err, user.ErrAdNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}

func TestAdService_TooMany(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	repo := inmem.NewAdRepository()
	ads := service.NewAdService(w.users, repo, repo, inmem.TxManager{}, admins)
	admin := w.person(t, "admin")

	for range user.MaxAds {
		if _, err := ads.Create(ctx, admin, user.AdDraft{URL: "https://a.example", Title: "A"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ads.Create(ctx, admin, user.AdDraft{URL: "https://a.example", Title: "A"}); !errors.Is(err, user.ErrTooManyAds) {
		t.Fatalf("one too many: %v", err)
	}
}
