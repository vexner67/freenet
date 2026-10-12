package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/service"
)

// An erased user reads nobody and is read by nobody: a page registered
// anew under the same ID would otherwise inherit the news.
func TestPurgeService_Sources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	posts := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{}})
	sources := service.NewSourceService(store.Sources())
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Sources: store.Sources(), Comments: store.Comments(),
		Suggestions: inmem.NewSuggestionRepository(), Topics: inmem.NewTopicRepository(), Albums: inmem.NewAlbumRepository(), Notes: inmem.NewNoteRepository(),
		Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{},
	})
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	for _, pair := range [][2]uuid.UUID{{ann, bob}, {bob, ann}, {cat, bob}} {
		if err := sources.Follow(ctx, pair[0], pair[1], false); err != nil {
			t.Fatal(err)
		}
	}
	// Told twice, a follow is one follow.
	if err := sources.Follow(ctx, cat, bob, false); err != nil {
		t.Fatal(err)
	}

	write(t, posts, bob, "запись Боба")

	if err := purge.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}

	// Ann, were she back, reads bob no more; cat still does.
	if page, err := posts.Feed(ctx, ann, service.FeedSources{}, uuid.Nil, 10); err != nil || len(page.Posts) != 0 {
		t.Fatalf("the feed of the erased = %v, %v", ids(page.Posts), err)
	}
	if page, err := posts.Feed(ctx, cat, service.FeedSources{}, uuid.Nil, 10); err != nil || len(page.Posts) != 1 {
		t.Fatalf("cat's feed = %v, %v", ids(page.Posts), err)
	}

	// Bob no longer reads ann either.
	write(t, posts, ann, "новая Анна")
	if page, err := posts.Feed(ctx, bob, service.FeedSources{}, uuid.Nil, 10); err != nil || len(page.Posts) != 1 {
		t.Fatalf("bob's feed = %v, %v", ids(page.Posts), err)
	}
}
