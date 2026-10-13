package service_test

import (
	"context"
	"errors"
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

// A stretch of sources that does not hold together is refused whole.
func TestSourceService_Sync(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sources := service.NewSourceService(inmem.NewStore().Sources())
	owner := uuid.New()
	low, high := uuid.MustParse("00000000-0000-7000-8000-000000000001"), uuid.MustParse("00000000-0000-7000-8000-000000000002")

	tests := []struct {
		name    string
		stretch service.SourceSync
		wantErr bool
	}{
		{name: "a stretch", stretch: service.SourceSync{Owner: owner, IDs: []uuid.UUID{low, high}}},
		{name: "the last, of nobody", stretch: service.SourceSync{Owner: owner, Last: true}},
		{name: "of a community", stretch: service.SourceSync{Owner: owner, Club: true, After: low, IDs: []uuid.UUID{high}, Last: true}},
		{name: "nobody's", stretch: service.SourceSync{IDs: []uuid.UUID{low}}, wantErr: true},
		{name: "empty and not the last", stretch: service.SourceSync{Owner: owner}, wantErr: true},
		{name: "out of order", stretch: service.SourceSync{Owner: owner, IDs: []uuid.UUID{high, low}}, wantErr: true},
		{name: "twice the same", stretch: service.SourceSync{Owner: owner, IDs: []uuid.UUID{low, low}}, wantErr: true},
		{name: "not above the start", stretch: service.SourceSync{Owner: owner, After: low, IDs: []uuid.UUID{low}}, wantErr: true},
		{name: "the owner among them", stretch: service.SourceSync{Owner: high, IDs: []uuid.UUID{low, high}}, wantErr: true},
		{name: "too many", stretch: service.SourceSync{Owner: owner, IDs: make([]uuid.UUID, service.MaxSyncIDs+1)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := sources.Sync(ctx, tt.stretch)
			if tt.wantErr != errors.Is(err, service.ErrSyncMalformed) || (!tt.wantErr && err != nil) {
				t.Fatalf("Sync = %v, want an error: %v", err, tt.wantErr)
			}
		})
	}
}
