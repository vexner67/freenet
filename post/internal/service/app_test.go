package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestAppService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	documents := inmem.NewDocumentRepository()
	docs := service.NewDocumentService(documents, fakeTx{}, &fakeDocumentKeeper{})
	svc := service.NewAppService(inmem.NewAppRepository(documents), documents)
	ann, bob := uuid.New(), uuid.New()

	snake := addDocument(t, docs, ann, "snake.html")
	tetris := addDocument(t, docs, ann, "tetris.html")
	pdf := addDocument(t, docs, ann, "a.pdf")

	if err := svc.Install(ctx, bob, pdf.ID()); !errors.Is(err, post.ErrGameNotFound) {
		t.Fatalf("Install of a pdf: %v", err)
	}
	if err := svc.Install(ctx, bob, uuid.New()); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Install of nothing: %v", err)
	}

	// Nobody added any yet: all of them is empty, own has both.
	if page, err := svc.List(ctx, bob, post.AppsAll, uuid.Nil, 10); err != nil || len(page.Apps) != 0 {
		t.Fatalf("all before = %+v, %v", page, err)
	}
	page, err := svc.List(ctx, ann, post.AppsOwn, uuid.Nil, 1)
	if err != nil || len(page.Apps) != 1 || page.Apps[0].Document.ID() != tetris.ID() || page.NextCursor != tetris.ID() {
		t.Fatalf("own = %+v, %v", page, err)
	}
	if page, err = svc.List(ctx, ann, post.AppsOwn, page.NextCursor, 1); err != nil || len(page.Apps) != 1 || page.Apps[0].Document.ID() != snake.ID() || page.NextCursor != uuid.Nil {
		t.Fatalf("own, the second page = %+v, %v", page, err)
	}

	// Twice changes nothing.
	for range 2 {
		if err = svc.Install(ctx, bob, snake.ID()); err != nil {
			t.Fatal(err)
		}
	}
	if err = svc.Install(ctx, ann, snake.ID()); err != nil {
		t.Fatal(err)
	}

	app, err := svc.Get(ctx, bob, snake.ID())
	if err != nil || app.Count != 2 || !app.Installed {
		t.Fatalf("Get = %+v, %v", app, err)
	}
	if app, err = svc.Get(ctx, uuid.Nil, tetris.ID()); err != nil || app.Count != 0 || app.Installed {
		t.Fatalf("Get as a guest = %+v, %v", app, err)
	}
	if _, err = svc.Get(ctx, bob, pdf.ID()); !errors.Is(err, post.ErrGameNotFound) {
		t.Fatalf("Get of a pdf: %v", err)
	}

	if page, err = svc.List(ctx, bob, post.AppsAll, uuid.Nil, 10); err != nil || len(page.Apps) != 1 || page.Apps[0].Count != 2 {
		t.Fatalf("all = %+v, %v", page, err)
	}
	if page, err = svc.List(ctx, bob, post.AppsInstalled, uuid.Nil, 10); err != nil || len(page.Apps) != 1 || !page.Apps[0].Installed {
		t.Fatalf("installed = %+v, %v", page, err)
	}

	if err = svc.Uninstall(ctx, bob, snake.ID()); err != nil {
		t.Fatal(err)
	}
	if page, err = svc.List(ctx, bob, post.AppsInstalled, uuid.Nil, 10); err != nil || len(page.Apps) != 0 {
		t.Fatalf("installed after = %+v, %v", page, err)
	}
	if app, _ = svc.Get(ctx, bob, snake.ID()); app.Count != 1 || app.Installed {
		t.Fatalf("Get after = %+v", app)
	}
}
