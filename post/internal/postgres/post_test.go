package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestPostRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewPostRepository(pool)
	author := uuid.New()

	var saved []*post.Post
	for _, text := range []string{"раз", "два", "три"} {
		p, err := post.New(author, post.Text(text), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
		saved = append(saved, p)
	}

	got, err := repo.Get(ctx, saved[0].ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if *got != *saved[0] {
		t.Fatalf("round trip: got %+v, want %+v", got, saved[0])
	}

	page, err := repo.ListByAuthor(ctx, author, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Text() != "три" || page[1].Text() != "два" {
		t.Fatalf("first page: %v", page)
	}

	rest, err := repo.ListByAuthor(ctx, author, page[1].ID(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0].Text() != "раз" {
		t.Fatalf("after cursor: %v", rest)
	}

	other := uuid.New()
	theirs, err := post.New(other, "чужая", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, theirs); err != nil {
		t.Fatal(err)
	}

	feed, err := repo.ListByAuthors(ctx, []uuid.UUID{author, other}, uuid.Nil, 10)
	if err != nil || len(feed) != 4 || feed[0].Text() != "чужая" || feed[3].Text() != "раз" {
		t.Fatalf("ListByAuthors = %v, %v", feed, err)
	}

	if n, err := repo.CountByAuthor(ctx, author); err != nil || n != 3 {
		t.Fatalf("CountByAuthor = %d, %v", n, err)
	}

	if err = repo.Delete(ctx, saved[0].ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = repo.Delete(ctx, saved[0].ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("second Delete: %v", err)
	}
	if _, err = repo.Get(ctx, saved[0].ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
}
