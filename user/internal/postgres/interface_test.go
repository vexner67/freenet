package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestInterface(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := user.New(user.Email(unique("ui") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	// A new user reads back with the site as it is by default.
	got, err := repo.GetByID(ctx, u.ID())
	if ui := got.Interface(); err != nil || ui.AvatarStyle != user.AvatarsSquare || ui.MainPage != user.MainPagePage || ui.HiddenMenu != nil {
		t.Fatalf("a new user = %+v, %v", ui, err)
	}

	ui, err := user.NewInterface("round", "feed", []string{"events", "photos"})
	if err != nil {
		t.Fatal(err)
	}
	got.UpdateInterface(ui)
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	got, err = repo.GetByID(ctx, u.ID())
	if saved := got.Interface(); err != nil || saved.AvatarStyle != user.AvatarsRound || saved.MainPage != user.MainPageFeed ||
		!slices.Equal(saved.HiddenMenu, []user.MenuItem{user.MenuPhotos, user.MenuEvents}) {
		t.Fatalf("after Update = %+v, %v", saved, err)
	}

	// A whole menu again is an empty list, not NULL.
	got.UpdateInterface(user.DefaultInterface())
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.Interface().HiddenMenu != nil || got.Interface().AvatarStyle != user.AvatarsSquare {
		t.Fatalf("back to default = %+v, %v", got.Interface(), err)
	}
}
