package service_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// postAt keeps a post of author written ago with likes, as if it had been
// written then.
func postAt(t *testing.T, store *inmem.Store, author uuid.UUID, text string, ago time.Duration, likes int) *post.Post {
	t.Helper()

	at := time.Now().Add(-ago).UTC()
	id := post.WindowStart(at)
	if _, err := rand.Read(id[6:]); err != nil {
		t.Fatal(err)
	}
	id[6] = id[6]&0x0f | 0x70
	id[8] = id[8]&0x3f | 0x80

	p := post.Restore(post.Stored{
		ID: id, WallID: author, AuthorID: author, Content: textContent(t, text), CreatedAt: at,
		Counters: post.Counters{Likes: likes},
	})
	if err := store.Posts().Save(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	return p
}

func newRankService(store *inmem.Store) (*service.PostService, follow) {
	sources := service.NewSourceService(store.Sources())
	reads := func(reader uuid.UUID, people []uuid.UUID, _ ...uuid.UUID) {
		for _, id := range people {
			if err := sources.Follow(context.Background(), reader, id, false); err != nil {
				panic(err)
			}
		}
	}

	return service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{}, Switches: store.Posts(), Polls: store.Polls()}), reads
}

func TestPostService_RankedFeed(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	svc, reads := newRankService(store)
	ctx := context.Background()
	ann, bob, reader := uuid.New(), uuid.New(), uuid.New()
	reads(reader, []uuid.UUID{ann, bob})

	postAt(t, store, ann, "old", 10*24*time.Hour, 100)
	postAt(t, store, ann, "older", 12*24*time.Hour, 0)
	postAt(t, store, bob, "fresh", time.Hour, 0)
	postAt(t, store, ann, "popular", 2*time.Hour, 30)
	postAt(t, store, bob, "liked a bit", 5*time.Hour, 5)

	var got []string
	cursor := service.FeedCursor{Ranked: true}
	for pages := 0; ; pages++ {
		page, err := svc.RankedFeed(ctx, reader, service.FeedSources{}, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) > 2 || pages > 5 {
			t.Fatalf("page of %d, %d pages", len(page.Posts), pages)
		}
		got = append(got, texts(page.Posts)...)
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}

	want := []string{"popular", "liked a bit", "fresh", "old", "older"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	// The plain news stay newest first.
	plain, err := svc.RankedFeed(ctx, reader, service.FeedSources{}, service.FeedCursor{}, 10)
	if err != nil || texts(plain.Posts)[0] != "fresh" {
		t.Fatalf("plain news: %v, %v", texts(plain.Posts), err)
	}
}

func TestPostService_RankedFeedAffinity(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	svc, reads := newRankService(store)
	ctx := context.Background()
	ann, bob, reader := uuid.New(), uuid.New(), uuid.New()
	reads(reader, []uuid.UUID{ann, bob})

	postAt(t, store, bob, "bob", 3*time.Hour, 0)
	postAt(t, store, ann, "ann", 3*time.Hour, 0)
	earlier := postAt(t, store, bob, "bob earlier", 20*24*time.Hour, 0)

	first, err := svc.RankedFeed(ctx, reader, service.FeedSources{}, service.FeedCursor{Ranked: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(first.Posts); got[0] != "ann" {
		t.Fatalf("before the like: %v", got)
	}

	if _, err := svc.Like(ctx, reader, earlier.ID()); err != nil {
		t.Fatal(err)
	}

	liked, err := svc.RankedFeed(ctx, reader, service.FeedSources{}, service.FeedCursor{Ranked: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(liked.Posts); got[0] != "bob" {
		t.Fatalf("who the reader likes comes first: %v", got)
	}
}
