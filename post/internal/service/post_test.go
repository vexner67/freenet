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

func newPostService() *service.PostService {
	store := inmem.NewStore()
	return service.NewPostService(store.Posts(), store.Likes())
}

func write(t *testing.T, svc *service.PostService, author uuid.UUID, texts ...string) []*post.Post {
	t.Helper()

	posts := make([]*post.Post, 0, len(texts))
	for _, text := range texts {
		p, err := svc.Create(context.Background(), author, post.Text(text))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		posts = append(posts, p)
	}

	return posts
}

func texts(posts []*post.Post) []string {
	out := make([]string, len(posts))
	for i, p := range posts {
		out[i] = p.Text().String()
	}
	return out
}

func TestPostService_Delete(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	author := uuid.New()
	p := write(t, svc, author, "привет")[0]

	if err := svc.Delete(ctx, uuid.New(), p.ID()); !errors.Is(err, post.ErrNotAuthor) {
		t.Fatalf("stranger: got %v, want ErrNotAuthor", err)
	}

	if err := svc.Delete(ctx, author, p.ID()); err != nil {
		t.Fatalf("author: %v", err)
	}

	if err := svc.Delete(ctx, author, p.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("twice: got %v, want ErrNotFound", err)
	}
}

func TestPostService_ListByAuthor(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	write(t, svc, ann, "1", "2", "3", "4", "5")
	write(t, svc, bob, "чужое")

	var (
		got    [][]string
		cursor uuid.UUID
	)

	for {
		page, err := svc.ListByAuthor(ctx, uuid.Nil, ann, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Fatalf("total = %d, want 5", page.Total)
		}

		got = append(got, texts(page.Posts))

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	want := [][]string{{"5", "4"}, {"3", "2"}, {"1"}}
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || got[i][0] != want[i][0] || got[i][len(got[i])-1] != want[i][len(want[i])-1] {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}

	t.Run("exact multiple of the page size ends without a cursor", func(t *testing.T) {
		page, err := svc.ListByAuthor(ctx, uuid.Nil, ann, uuid.Nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 || page.NextCursor != uuid.Nil {
			t.Fatalf("posts = %d, next = %s", len(page.Posts), page.NextCursor)
		}
	})

	t.Run("bad size falls back to the default", func(t *testing.T) {
		page, err := svc.ListByAuthor(ctx, uuid.Nil, ann, uuid.Nil, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 {
			t.Fatalf("posts = %d", len(page.Posts))
		}
	})

	t.Run("empty wall", func(t *testing.T) {
		page, err := svc.ListByAuthor(ctx, uuid.Nil, uuid.New(), uuid.Nil, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 0 || page.Total != 0 || page.NextCursor != uuid.Nil {
			t.Fatalf("page = %+v", page)
		}
	})
}

func TestPostService_Feed(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	write(t, svc, ann, "ann 1")
	write(t, svc, bob, "bob 1")
	write(t, svc, cat, "cat: not followed")
	write(t, svc, ann, "ann 2")

	first, err := svc.Feed(ctx, uuid.Nil, []uuid.UUID{ann, bob}, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(first.Posts); len(got) != 2 || got[0] != "ann 2" || got[1] != "bob 1" || first.NextCursor == uuid.Nil {
		t.Fatalf("first page: %v", got)
	}

	rest, err := svc.Feed(ctx, uuid.Nil, []uuid.UUID{ann, bob}, first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(rest.Posts); len(got) != 1 || got[0] != "ann 1" || rest.NextCursor != uuid.Nil {
		t.Fatalf("last page: %v", got)
	}

	empty, err := svc.Feed(ctx, uuid.Nil, nil, uuid.Nil, 20)
	if err != nil || len(empty.Posts) != 0 {
		t.Fatalf("no authors: %+v, %v", empty, err)
	}

	if _, err = svc.Feed(ctx, uuid.Nil, make([]uuid.UUID, service.MaxFeedAuthors+1), uuid.Nil, 20); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many authors: %v", err)
	}
}

func TestPostService_Likes(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	posts := write(t, svc, ann, "первая", "вторая")
	first, second := posts[0], posts[1]

	steps := []struct {
		name string
		do   func(ctx context.Context, userID, postID uuid.UUID) (int, error)
		user uuid.UUID
		want int
	}{
		{"ann likes her post", svc.Like, ann, 1},
		{"bob likes it too", svc.Like, bob, 2},
		{"a second like counts once", svc.Like, bob, 2},
		{"bob takes his back", svc.Unlike, bob, 1},
		{"unliking twice changes nothing", svc.Unlike, bob, 1},
	}

	for _, step := range steps {
		likes, err := step.do(ctx, step.user, first.ID())
		if err != nil || likes != step.want {
			t.Fatalf("%s: likes = %d, %v; want %d", step.name, likes, err, step.want)
		}
	}

	p, liked, err := svc.Get(ctx, ann, first.ID())
	if err != nil || !liked || p.Counters().Likes != 1 {
		t.Fatalf("Get by ann = %v, %v, %v", p.Counters(), liked, err)
	}
	if _, liked, _ = svc.Get(ctx, bob, first.ID()); liked {
		t.Fatal("bob took his like back")
	}
	if _, liked, _ = svc.Get(ctx, uuid.Nil, first.ID()); liked {
		t.Fatal("an anonymous viewer likes nothing")
	}

	wall, err := svc.ListByAuthor(ctx, ann, ann, uuid.Nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !wall.Liked[first.ID()] || wall.Liked[second.ID()] {
		t.Fatalf("wall liked = %v", wall.Liked)
	}

	feed, err := svc.Feed(ctx, ann, []uuid.UUID{ann}, uuid.Nil, 20)
	if err != nil || !feed.Liked[first.ID()] || len(feed.Liked) != 1 {
		t.Fatalf("feed liked = %v, %v", feed.Liked, err)
	}

	anonymous, err := svc.ListByAuthor(ctx, uuid.Nil, ann, uuid.Nil, 20)
	if err != nil || len(anonymous.Liked) != 0 {
		t.Fatalf("anonymous liked = %v, %v", anonymous.Liked, err)
	}

	if _, err = svc.Like(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("like a missing post: %v", err)
	}
	if _, err = svc.Unlike(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("unlike a missing post: %v", err)
	}
	if _, _, err = svc.Get(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("get a missing post: %v", err)
	}
}
