package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestPostRepository_Update(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	posts := postgres.NewPostRepository(newPool(t))
	author := uuid.New()
	photo := uuid.New()

	p, err := post.New(author, author, post.RestoreContent("было", post.Photos{uuid.New()}, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	edited, err := p.Edited(author, "стало", post.Photos{photo}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Update(ctx, edited); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := posts.Get(ctx, p.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Content().Text() != "стало" || !slices.Equal(got.Content().Photos(), post.Photos{photo}) || !got.EditedAt().Equal(edited.EditedAt()) {
		t.Fatalf("stored = %+v, edited at %v; want %v", got.Content(), got.EditedAt(), edited.EditedAt())
	}

	// Photos taken away leave an empty array, not NULL.
	if edited, err = p.Edited(author, "без фото", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = posts.Update(ctx, edited); err != nil {
		t.Fatalf("Update without photos: %v", err)
	}

	missing, err := post.New(author, author, post.RestoreContent("нет", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Update(ctx, missing); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("Update of a missing post: %v", err)
	}
}

func TestPostRepository_Pin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	posts := postgres.NewPostRepository(newPool(t))
	owner, guest := uuid.New(), uuid.New()

	save := func(author uuid.UUID, text string) *post.Post {
		t.Helper()

		p, err := post.New(owner, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	first, byGuest, last := save(owner, "первая"), save(guest, "гостя"), save(owner, "последняя")

	if _, err := posts.Pinned(ctx, owner); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("nothing pinned yet: %v", err)
	}

	for range 2 {
		if err := posts.Pin(ctx, first.ID()); err != nil {
			t.Fatalf("Pin: %v", err)
		}
	}

	pinned, err := posts.Pinned(ctx, owner)
	if err != nil || pinned.ID() != first.ID() || !pinned.Pinned() {
		t.Fatalf("Pinned = %v, %v", pinned, err)
	}
	if got, _ := posts.Get(ctx, first.ID()); !got.Pinned() {
		t.Fatal("Get must tell the post is pinned")
	}

	wall, err := posts.ListByWall(ctx, owner, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ids := postIDs(wall); !slices.Equal(ids, []uuid.UUID{last.ID(), byGuest.ID()}) {
		t.Fatalf("wall without the pinned post = %v", ids)
	}

	// Another post replaces the pin.
	if err = posts.Pin(ctx, byGuest.ID()); err != nil {
		t.Fatal(err)
	}
	if got, _ := posts.Get(ctx, first.ID()); got.Pinned() {
		t.Fatal("the post pinned before must not be any more")
	}
	if pinned, err = posts.Pinned(ctx, owner); err != nil || pinned.ID() != byGuest.ID() {
		t.Fatalf("Pinned after replacing = %v, %v", pinned, err)
	}

	for range 2 {
		if err = posts.Unpin(ctx, byGuest.ID()); err != nil {
			t.Fatalf("Unpin: %v", err)
		}
	}
	if _, err = posts.Pinned(ctx, owner); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("after Unpin: %v", err)
	}

	// A deleted post takes its pin with it.
	if err = posts.Pin(ctx, last.ID()); err != nil {
		t.Fatal(err)
	}
	if err = posts.Delete(ctx, last.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = posts.Pinned(ctx, owner); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("after deleting the pinned post: %v", err)
	}
	if err = posts.Pin(ctx, last.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("Pin of a deleted post: %v", err)
	}
}

func postIDs(posts []*post.Post) []uuid.UUID {
	ids := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		ids[i] = p.ID()
	}
	return ids
}
