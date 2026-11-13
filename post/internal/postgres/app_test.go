package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestAppRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	documents := postgres.NewDocumentRepository(pool)
	apps := postgres.NewAppRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	var games []*post.Document
	for _, name := range []string{"snake.html", "tetris.html"} {
		d, err := post.NewDocument(uuid.Must(uuid.NewV7()), ann, post.DocumentName(name), post.DocumentHTML, 100, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = documents.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
		games = append(games, d)
	}
	pdf, _ := post.NewDocument(uuid.Must(uuid.NewV7()), ann, "a.pdf", "pdf", 100, now)
	if err := documents.Save(ctx, pdf); err != nil {
		t.Fatal(err)
	}

	own, err := apps.List(ctx, post.AppsOwn, ann, uuid.Nil, 10)
	if err != nil || len(own) != 2 || own[0].ID() != games[1].ID() {
		t.Fatalf("own = %v, %v", own, err)
	}
	if own, err = apps.List(ctx, post.AppsOwn, ann, games[1].ID(), 10); err != nil || len(own) != 1 || own[0].ID() != games[0].ID() {
		t.Fatalf("own after the cursor = %v, %v", own, err)
	}

	for _, user := range []uuid.UUID{bob, bob, ann} {
		if err = apps.Install(ctx, user, games[0].ID(), now); err != nil {
			t.Fatalf("Install: %v", err)
		}
	}

	all, err := apps.List(ctx, post.AppsAll, bob, uuid.Nil, 10)
	if err != nil || len(all) != 1 || all[0].ID() != games[0].ID() {
		t.Fatalf("all = %v, %v", all, err)
	}
	if installed, err := apps.List(ctx, post.AppsInstalled, bob, uuid.Nil, 10); err != nil || len(installed) != 1 {
		t.Fatalf("installed = %v, %v", installed, err)
	}

	users, err := apps.Users(ctx, bob, []uuid.UUID{games[0].ID(), games[1].ID()})
	if err != nil || users[games[0].ID()].Count != 2 || !users[games[0].ID()].Installed || users[games[1].ID()].Count != 0 {
		t.Fatalf("Users = %+v, %v", users, err)
	}
	if users, err = apps.Users(ctx, uuid.Nil, []uuid.UUID{games[0].ID()}); err != nil || users[games[0].ID()].Installed {
		t.Fatalf("Users of a guest = %+v, %v", users, err)
	}

	if err = apps.Uninstall(ctx, bob, games[0].ID()); err != nil {
		t.Fatal(err)
	}
	if err = apps.DeleteByUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if all, err = apps.List(ctx, post.AppsAll, bob, uuid.Nil, 10); err != nil || len(all) != 0 {
		t.Fatalf("all after = %v, %v", all, err)
	}
}
