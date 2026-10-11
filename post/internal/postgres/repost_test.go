package postgres_test

import (
	"bytes"
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
			content, err := post.NewRepost(original.ID(), "", nil)
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

	// Who shared it, newest first, a page at a time; an archived repost
	// only when asked for. A repost keeps the photos added to it.
	list, err := repo.ListReposts(ctx, original.ID(), uuid.Nil, n, false)
	if err != nil || len(list) != n-1 {
		t.Fatalf("ListReposts = %d, %v; want %d", len(list), err, n-1)
	}
	for i := 1; i < len(list); i++ {
		a, b := list[i-1].ID(), list[i].ID()
		if bytes.Compare(a[:], b[:]) <= 0 {
			t.Fatalf("reposts are not newest first at %d", i)
		}
	}
	rest, err := repo.ListReposts(ctx, original.ID(), list[2].ID(), 3, false)
	if err != nil || len(rest) != 3 || rest[0].ID() != list[3].ID() {
		t.Fatalf("ListReposts after a cursor = %d, %v", len(rest), err)
	}
	if err = repo.SetArchived(ctx, list[0].ID(), true); err != nil {
		t.Fatal(err)
	}
	if shown, err := repo.ListReposts(ctx, original.ID(), uuid.Nil, n, false); err != nil || len(shown) != n-2 {
		t.Fatalf("ListReposts without the archived = %d, %v", len(shown), err)
	}
	if all, err := repo.ListReposts(ctx, original.ID(), uuid.Nil, n, true); err != nil || len(all) != n-1 {
		t.Fatalf("ListReposts with the archived = %d, %v", len(all), err)
	}
	if err = repo.SetArchived(ctx, list[0].ID(), false); err != nil {
		t.Fatal(err)
	}

	photos := post.Photos{uuid.New(), uuid.New()}
	shown, err := post.NewRepost(original.ID(), "", photos)
	if err != nil {
		t.Fatal(err)
	}
	pictured, err := post.New(ann, ann, shown, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, pictured); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, pictured.ID()); err != nil || !reflect.DeepEqual(got, pictured) {
		t.Fatalf("a repost with photos round trip: %+v, %v; want %+v", got, err, pictured)
	}
	if err = repo.Delete(ctx, pictured.ID()); err != nil {
		t.Fatal(err)
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

	content, err := post.NewRepost(original.ID(), "поздно", nil)
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
