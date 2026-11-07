package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestInterface(t *testing.T) {
	repo, pool := newRepo(t)

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
	if ui := got.Interface(); err != nil || !reflect.DeepEqual(ui, user.DefaultInterface()) {
		t.Fatalf("a new user = %+v, %v", ui, err)
	}

	offset := -210
	ui, err := user.NewInterface(user.InterfaceForm{
		AvatarStyle: "round", Theme: "dark", HideRating: true, NSFW: "hide", PostView: "microblog",
		MainPage: "feed", Timezone: &offset, HiddenMenu: []string{"groups", "feed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = got.UpdateInterface(ui, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	got, err = repo.GetByID(ctx, u.ID())
	if saved := got.Interface(); err != nil || !reflect.DeepEqual(saved, ui) {
		t.Fatalf("after Update = %+v, %v; want %+v", saved, err, ui)
	}

	// A whole menu again is an empty list and the browser's clock a NULL.
	if err = got.UpdateInterface(user.DefaultInterface(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || !reflect.DeepEqual(got.Interface(), user.DefaultInterface()) {
		t.Fatalf("back to default = %+v, %v", got.Interface(), err)
	}

	// A link stored before it left the menu (the albums' "photos") is not
	// read back.
	if _, err = pool.Exec(ctx, `UPDATE users SET hidden_menu = ARRAY['photos', 'groups'] WHERE id = $1`, u.ID()); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || !reflect.DeepEqual(got.Interface().HiddenMenu, []user.MenuItem{user.MenuGroups}) {
		t.Fatalf("a link gone = %+v, %v", got.Interface().HiddenMenu, err)
	}
}
