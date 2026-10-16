package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestPostRepository_Link(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	posts := postgres.NewPostRepository(newPool(t))
	ann := uuid.New()

	save := func(link post.Link) *post.Post {
		t.Helper()

		content, err := post.NewContent("Смотрите: https://example.com/a", nil)
		if err != nil {
			t.Fatal(err)
		}

		p, err := post.New(ann, ann, content.WithLink(link), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return p
	}

	card, err := post.NewLink(post.LinkDraft{
		URL: "https://example.com/a", Title: "Пример", Description: "Страница примера.", Site: "Example", PhotoID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}

	carded := save(card)
	read, err := posts.Get(ctx, carded.ID())
	if err != nil || read.Content().Link() != card {
		t.Fatalf("the card read back = %+v, %v; want %+v", read.Content().Link(), err, card)
	}

	// An edit keeps the card.
	edited, err := read.Edited(ann, "Другие слова", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if read, err = posts.Get(ctx, carded.ID()); err != nil || read.Content().Link() != card {
		t.Fatalf("the card after an edit = %+v, %v", read.Content().Link(), err)
	}

	bare := save(post.Link{})
	if read, err = posts.Get(ctx, bare.ID()); err != nil || !read.Content().Link().IsZero() {
		t.Fatalf("a post without a card reads %+v, %v", read.Content().Link(), err)
	}

	// A card without a picture.
	plain, err := post.NewLink(post.LinkDraft{URL: "http://example.com", Title: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	if read, err = posts.Get(ctx, save(plain).ID()); err != nil || read.Content().Link() != plain {
		t.Fatalf("a card without a picture reads %+v, %v", read.Content().Link(), err)
	}
}
