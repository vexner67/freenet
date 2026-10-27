package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestArticleRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	articles := postgres.NewArticleRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	write := func(author uuid.UUID, d post.ArticleDraft) *post.Article {
		t.Helper()

		a, err := post.NewArticle(author, d, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = articles.Save(ctx, a); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return a
	}

	text := []post.Block{{Kind: post.BlockParagraph, Spans: []post.Span{{Text: "Текст"}}}}
	first := write(ann, post.ArticleDraft{Title: "Первая", CoverID: uuid.New(), Blocks: []post.Block{
		{Kind: post.BlockHeading, Spans: []post.Span{{Text: "Глава"}}},
		{Kind: post.BlockParagraph, Spans: []post.Span{{Text: "Это "}, {Text: "важно", Bold: true, Italic: true}, {Text: "ссылка", Link: "https://example.com"}}},
		{Kind: post.BlockPhoto, PhotoID: uuid.New(), Caption: "Подпись"},
		{Kind: post.BlockDivider},
	}})
	second := write(ann, post.ArticleDraft{Title: "Вторая", Blocks: text})
	theirs := write(bob, post.ArticleDraft{Title: "Чужая", Blocks: text})

	got, err := articles.Get(ctx, first.ID())
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, first)
	}
	if _, err = articles.Get(ctx, uuid.New()); !errors.Is(err, post.ErrArticleNotFound) {
		t.Fatalf("Get a missing one: %v", err)
	}

	list, err := articles.List(ctx, ann, uuid.Nil, 10)
	if err != nil || len(list) != 2 || list[0].ID() != second.ID() || list[1].ID() != first.ID() {
		t.Fatalf("List = %v, %v", list, err)
	}
	if list, err = articles.List(ctx, ann, second.ID(), 10); err != nil || len(list) != 1 || list[0].ID() != first.ID() {
		t.Fatalf("List after the newest = %v, %v", list, err)
	}
	if n, err := articles.Count(ctx, ann); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	edited, err := first.Edited(ann, post.ArticleDraft{Title: "Новая", Blocks: text}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = articles.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if got, err = articles.Get(ctx, first.ID()); err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("Get the edited = %+v, %v; want %+v", got, err, edited)
	}

	if err = articles.Delete(ctx, second.ID()); err != nil {
		t.Fatal(err)
	}
	if err = articles.Delete(ctx, second.ID()); !errors.Is(err, post.ErrArticleNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}

	if err = articles.DeleteByUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if n, err := articles.Count(ctx, ann); err != nil || n != 0 {
		t.Fatalf("Count after DeleteByUser = %d, %v", n, err)
	}
	if _, err = articles.Get(ctx, theirs.ID()); err != nil {
		t.Fatalf("another's article: %v", err)
	}
}
