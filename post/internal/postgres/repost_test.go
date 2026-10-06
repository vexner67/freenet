package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestPostRepository_Reposts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	ann := uuid.New()

	original, err := post.New(ann, ann, post.RestoreContent("оригинал", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, original); err != nil {
		t.Fatal(err)
	}

	reposts := func() int {
		t.Helper()

		got, err := repo.Get(ctx, original.ID())
		if err != nil {
			t.Fatal(err)
		}

		return got.Counters().Reposts
	}

	// Concurrent reposts are all counted.
	const n = 10

	var (
		wg    sync.WaitGroup
		saved = make([]*post.Post, n)
		errs  = make([]error, n)
	)
	for i := range n {
		wg.Go(func() {
			by := uuid.New()
			content, err := post.NewRepost(original.ID(), "")
			if err == nil {
				saved[i], err = post.New(by, by, content, time.Now())
			}
			if err == nil {
				err = repo.Save(ctx, saved[i])
			}
			errs[i] = err
		})
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("Save repost: %v", err)
		}
	}
	if got := reposts(); got != n {
		t.Fatalf("reposts = %d, want %d", got, n)
	}

	got, err := repo.Get(ctx, saved[0].ID())
	if err != nil || !reflect.DeepEqual(got, saved[0]) {
		t.Fatalf("a repost without a word round trip: %+v, %v; want %+v", got, err, saved[0])
	}

	if err = repo.Delete(ctx, saved[0].ID()); err != nil {
		t.Fatal(err)
	}
	if got := reposts(); got != n-1 {
		t.Fatalf("after deleting a repost: reposts = %d, want %d", got, n-1)
	}

	// The shared post goes; its reposts stay and can still be deleted.
	if err = repo.Delete(ctx, original.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, saved[1].ID()); err != nil {
		t.Fatalf("a repost outlives the shared post: %v", err)
	}
	if err = repo.Delete(ctx, saved[1].ID()); err != nil {
		t.Fatalf("deleting a repost of a deleted post: %v", err)
	}

	content, err := post.NewRepost(original.ID(), "поздно")
	if err != nil {
		t.Fatal(err)
	}
	late, err := post.New(ann, ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, late); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("sharing a deleted post: %v, want ErrNotFound", err)
	}
	if _, err = repo.Get(ctx, late.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("the failed repost was saved: %v", err)
	}
}
