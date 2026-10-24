package postgres_test

import (
	"context"
	"crypto/rand"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestRankFeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	repo := postgres.NewPostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	now := time.Now().UTC()

	write := func(author uuid.UUID, text string, ago time.Duration, liked int) *post.Post {
		t.Helper()

		at := now.Add(-ago)
		id := post.WindowStart(at)
		if _, err := rand.Read(id[6:]); err != nil {
			t.Fatal(err)
		}
		id[6] = id[6]&0x0f | 0x70
		id[8] = id[8]&0x3f | 0x80

		p := post.Restore(post.Stored{ID: id, WallID: author, AuthorID: author, Content: post.RestoreContent(post.Text(text), nil, uuid.Nil), CreatedAt: at})
		if err := repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		for range liked {
			if _, err := likes.Like(ctx, p.ID(), uuid.New()); err != nil {
				t.Fatal(err)
			}
		}

		return p
	}

	write(ann, "old", 10*24*time.Hour, 3)
	earlier := write(bob, "bob earlier", 20*24*time.Hour, 0)
	write(bob, "fresh", time.Hour, 0)
	write(ann, "popular", 2*time.Hour, 30)
	write(bob, "liked a bit", 5*time.Hour, 5)
	write(ann, "ann", 3*time.Hour, 0)

	sources := reader(t, []uuid.UUID{ann, bob})

	// What the inmem store ranks, the database ranks the same way: the
	// week only, page by page.
	var got []string
	first, err := repo.RankFeed(ctx, sources, now, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range first {
		got = append(got, string(r.Post.Content().Text()))
	}
	last := first[len(first)-1]
	rest, err := repo.RankFeed(ctx, sources, now, &service.RankMark{Score: last.Score, ID: last.Post.ID()}, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rest {
		got = append(got, string(r.Post.Content().Text()))
	}

	want := []string{"popular", "liked a bit", "fresh", "ann"}
	if !slices.Equal(got, want) {
		t.Fatalf("ranked %v, want %v", got, want)
	}

	// The reader likes bob: his posts score more.
	freshScore := func() float64 {
		t.Helper()
		ranked, err := repo.RankFeed(ctx, sources, now, nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range ranked {
			if r.Post.Content().Text() == "fresh" {
				return r.Score
			}
		}
		t.Fatal("no fresh post")
		return 0
	}
	before := freshScore()
	if _, err := likes.Like(ctx, earlier.ID(), sources.Reader); err != nil {
		t.Fatal(err)
	}
	if after := freshScore(); after <= before {
		t.Fatalf("the score of bob's post went from %v to %v", before, after)
	}
}
