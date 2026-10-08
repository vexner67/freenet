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

func TestSuggestionRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := postgres.NewSuggestionRepository(newPool(t))
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()

	suggest := func(author uuid.UUID, text post.Text, photos post.Photos) *post.Suggestion {
		t.Helper()
		s, err := post.NewSuggestion(club, author, post.RestoreContent(text, photos, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, s); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return s
	}

	first := suggest(ann, "", post.Photos{uuid.New(), uuid.New()})
	second := suggest(ann, "слова", nil)
	bobs := suggest(bob, "от Петра", nil)

	got, err := repo.Get(ctx, first.ID())
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, first)
	}
	if _, err = repo.Get(ctx, uuid.New()); !errors.Is(err, post.ErrSuggestionNotFound) {
		t.Fatalf("Get missing: %v", err)
	}

	all, err := repo.List(ctx, club, uuid.Nil, uuid.Nil, 10)
	if err != nil || len(all) != 3 || all[0].ID() != bobs.ID() || all[2].ID() != first.ID() {
		t.Fatalf("List all = %v, %v", all, err)
	}
	anns, err := repo.List(ctx, club, ann, second.ID(), 10)
	if err != nil || len(anns) != 1 || anns[0].ID() != first.ID() {
		t.Fatalf("ann's after the cursor = %v, %v", anns, err)
	}
	if n, err := repo.Count(ctx, club, ann); err != nil || n != 2 {
		t.Fatalf("Count ann's = %d, %v", n, err)
	}
	if n, err := repo.Count(ctx, club, uuid.Nil); err != nil || n != 3 {
		t.Fatalf("Count all = %d, %v", n, err)
	}

	if err = repo.Delete(ctx, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Delete(ctx, bobs.ID()); !errors.Is(err, post.ErrSuggestionNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}

	// A published suggestion reads back as the community's.
	posts := postgres.NewPostRepository(newPool(t))
	published, err := second.Published("слова!", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, published); err != nil {
		t.Fatal(err)
	}
	if got, err := posts.Get(ctx, published.ID()); err != nil || !got.Suggested() || !got.FromWall() || !got.Signed() {
		t.Fatalf("the published post = %+v, %v", got, err)
	}

	before, _ := repo.Count(ctx, club, uuid.Nil)
	deleted, err := repo.DeleteByUser(ctx, ann)
	if err != nil || len(deleted) != before || before == 0 {
		t.Fatalf("DeleteByUser = %v, %v; %d suggested", deleted, err, before)
	}
	if n, _ := repo.Count(ctx, club, uuid.Nil); n != 0 {
		t.Fatalf("left after DeleteByUser = %d", n)
	}
}
