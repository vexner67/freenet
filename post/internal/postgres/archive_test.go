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

func TestPostRepository_ArchiveSearchAndComments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	ann := uuid.New()

	var saved []*post.Post
	for _, text := range []string{"Еду на море", "горы", "МОРЕ волнуется", "скидка 50%", "под_черк"} {
		p, err := post.New(ann, ann, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		saved = append(saved, p)
	}

	list := func(filter post.WallFilter, q string) []string {
		t.Helper()

		pick := service.WallPick{Filter: filter, Query: post.WallQuery(q)}
		posts, err := repo.ListByWall(ctx, ann, pick, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("ListByWall(%q, %q): %v", filter, q, err)
		}
		if n, err := repo.CountByWall(ctx, ann, pick); err != nil || n != len(posts) {
			t.Fatalf("CountByWall(%q, %q) = %d, %v; listed %d", filter, q, n, err, len(posts))
		}

		out := make([]string, len(posts))
		for i, p := range posts {
			out[i] = p.Content().Text().String()
		}

		return out
	}
	equal := func(got []string, want ...string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}

		return true
	}

	// Search: any case, the wildcards of LIKE as themselves.
	if got := list(post.WallAll, "море"); !equal(got, "МОРЕ волнуется", "Еду на море") {
		t.Fatalf("море = %v", got)
	}
	if got := list(post.WallAll, "%"); !equal(got, "скидка 50%") {
		t.Fatalf("%% = %v", got)
	}
	if got := list(post.WallAll, "_"); !equal(got, "под_черк") {
		t.Fatalf("_ = %v", got)
	}
	if got := list(post.WallAll, "снег"); len(got) != 0 {
		t.Fatalf("снег = %v", got)
	}

	// A pinned post is left out of all posts, but found by a search.
	if err := repo.Pin(ctx, saved[0].ID()); err != nil {
		t.Fatal(err)
	}
	if posts, _ := repo.ListByWall(ctx, ann, service.WallPick{}, uuid.Nil, 10); len(posts) != 4 {
		t.Fatalf("all posts but the pinned one: %d", len(posts))
	}
	if posts, _ := repo.ListByWall(ctx, ann, service.WallPick{Query: "еду"}, uuid.Nil, 10); len(posts) != 1 || !posts[0].Pinned() {
		t.Fatalf("the pinned post is not found: %d", len(posts))
	}

	// Archived, a post leaves the wall, the feed and its pin, and the count
	// of the site, which other tests change too: only the least is sure.
	if n, err := repo.Count(ctx); err != nil || n < len(saved) {
		t.Fatalf("Count = %d, %v; want these %d at least", n, err, len(saved))
	}
	for range 2 {
		if err := repo.SetArchived(ctx, saved[0].ID(), true); err != nil {
			t.Fatalf("SetArchived: %v", err)
		}
	}
	if err := repo.SetArchived(ctx, uuid.New(), true); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("archiving no post: %v", err)
	}
	if got, err := repo.Get(ctx, saved[0].ID()); err != nil || !got.Archived() || got.Pinned() {
		t.Fatalf("the archived post = %+v, %v", got, err)
	}
	if _, err := repo.Pinned(ctx, ann); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("the wall still pins it: %v", err)
	}
	if got := list(post.WallAll, ""); len(got) != 4 {
		t.Fatalf("the wall = %v", got)
	}
	if got := list(post.WallOwner, ""); len(got) != 4 {
		t.Fatalf("the owner's posts = %v", got)
	}
	if got := list(post.WallArchive, ""); !equal(got, "Еду на море") {
		t.Fatalf("the archive = %v", got)
	}
	if got := list(post.WallArchive, "горы"); len(got) != 0 {
		t.Fatalf("горы in the archive = %v", got)
	}
	if feed, err := repo.ListFeed(ctx, service.FeedSources{Authors: []uuid.UUID{ann}, Walls: []uuid.UUID{ann}}, uuid.Nil, 10); err != nil || len(feed) != 4 {
		t.Fatalf("the feed shows %d posts, %v", len(feed), err)
	}

	if err := repo.SetArchived(ctx, saved[0].ID(), false); err != nil {
		t.Fatalf("SetArchived(false): %v", err)
	}
	if err := repo.SetArchived(ctx, uuid.New(), false); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("returning no post: %v", err)
	}
	if got := list(post.WallAll, ""); len(got) != 5 || got[4] != "Еду на море" {
		t.Fatalf("the wall after the return = %v", got)
	}

	// Closed comments.
	if err := repo.SetCommentsClosed(ctx, saved[1].ID(), true); err != nil {
		t.Fatalf("SetCommentsClosed: %v", err)
	}
	if got, err := repo.Get(ctx, saved[1].ID()); err != nil || !got.CommentsClosed() {
		t.Fatalf("the closed post = %+v, %v", got, err)
	}
	if err := repo.SetCommentsClosed(ctx, saved[1].ID(), false); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(ctx, saved[1].ID()); got.CommentsClosed() {
		t.Fatal("the comments stay closed")
	}
	if err := repo.SetCommentsClosed(ctx, uuid.New(), true); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("closing the comments of no post: %v", err)
	}
}

func TestPostRepository_NSFW(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	ann := uuid.New()

	plain, err := post.New(ann, ann, post.RestoreContent("котики", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	adult, err := post.New(ann, ann, post.RestoreContent("для взрослых", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	adult = adult.Marked(true)

	for _, p := range []*post.Post{plain, adult} {
		if err = repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := repo.Get(ctx, adult.ID()); err != nil || !got.NSFW() {
		t.Fatalf("the adult post = %+v, %v", got, err)
	}
	if got, err := repo.Get(ctx, plain.ID()); err != nil || got.NSFW() {
		t.Fatalf("the plain post = %+v, %v", got, err)
	}

	sources := service.FeedSources{Authors: []uuid.UUID{ann}}
	if feed, err := repo.ListFeed(ctx, sources, uuid.Nil, 10); err != nil || len(feed) != 2 {
		t.Fatalf("the feed shows %d posts, %v", len(feed), err)
	}
	sources.HideNSFW = true
	if feed, err := repo.ListFeed(ctx, sources, uuid.Nil, 10); err != nil || len(feed) != 1 || feed[0].ID() != plain.ID() {
		t.Fatalf("the feed without adult posts shows %d, %v", len(feed), err)
	}

	// What the people the reader blacklisted wrote is left out, of the
	// news of everyone too.
	sources.Hidden = []uuid.UUID{ann}
	if feed, err := repo.ListFeed(ctx, sources, uuid.Nil, 10); err != nil || len(feed) != 0 {
		t.Fatalf("the feed without the blacklisted shows %d, %v", len(feed), err)
	}
	if feed, err := repo.ListFeed(ctx, service.FeedSources{Everyone: true, Hidden: []uuid.UUID{ann}}, uuid.Nil, 1000); err != nil ||
		slices.ContainsFunc(feed, func(p *post.Post) bool { return p.AuthorID() == ann }) {
		t.Fatalf("the feed of everyone shows the blacklisted, %v", err)
	}

	// An edit saves the mark with the text.
	edited, err := adult.Edited(ann, "для всех", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, edited.Marked(false)); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, adult.ID()); err != nil || got.NSFW() || got.Content().Text() != "для всех" {
		t.Fatalf("the edited post = %+v, %v", got, err)
	}
}
