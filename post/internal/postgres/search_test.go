package postgres_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestPostRepository_Search(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	ann, bob := uuid.New(), uuid.New()
	// The database is shared: a word of this test alone.
	word := "w" + strings.ReplaceAll(uuid.NewString(), "-", "")

	write := func(author uuid.UUID, text string) *post.Post {
		t.Helper()

		p, err := post.New(author, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return p
	}

	tagged := write(ann, "В поход! #"+strings.ToUpper(word))
	longer := write(bob, "#"+word+"s и лес")
	said := write(bob, "без тега, просто "+word)

	search := func(query string, sources service.FeedSources) []uuid.UUID {
		t.Helper()

		q, err := post.NewSearchQuery(query)
		if err != nil {
			t.Fatal(err)
		}

		posts, err := repo.Search(ctx, q, sources, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}

		count, err := repo.CountSearch(ctx, q, sources)
		if err != nil {
			t.Fatalf("CountSearch(%q): %v", query, err)
		}

		var found []uuid.UUID
		for _, p := range posts {
			found = append(found, p.ID())
		}
		if count != len(found) {
			t.Fatalf("CountSearch(%q) = %d, Search found %d", query, count, len(found))
		}

		return found
	}

	everyone := service.FeedSources{Everyone: true}

	if got := search("#"+word, everyone); !slices.Equal(got, []uuid.UUID{tagged.ID()}) {
		t.Fatalf("the tag finds %v; want only the post that says it", got)
	}
	if got := search(strings.ToUpper(word), everyone); !slices.Equal(got, []uuid.UUID{said.ID(), longer.ID(), tagged.ID()}) {
		t.Fatalf("the word finds %v; want every post, newest first", got)
	}
	if got := search(word, service.FeedSources{Everyone: true, Hidden: []uuid.UUID{bob}}); !slices.Equal(got, []uuid.UUID{tagged.ID()}) {
		t.Fatalf("without the blacklisted, the word finds %v", got)
	}

	// An edit tells the post's new tags.
	edited, err := tagged.Edited(ann, "теперь без тегов", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, edited); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := search("#"+word, everyone); len(got) != 0 {
		t.Fatalf("after the edit the tag finds %v", got)
	}

	if err = repo.SetArchived(ctx, said.ID(), true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	if got := search(word, everyone); !slices.Equal(got, []uuid.UUID{longer.ID()}) {
		t.Fatalf("the archived post is found: %v", got)
	}
}
