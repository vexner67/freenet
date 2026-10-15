package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type storyFixture struct {
	svc    *service.StoryService
	store  *inmem.Store
	photos *fakePhotos
	now    time.Time
}

func newStoryFixture() *storyFixture {
	f := &storyFixture{store: inmem.NewStore(), photos: &fakePhotos{}, now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	f.svc = service.NewStoryService(service.StoryDeps{
		Stories: f.store.Stories(), Views: f.store.StoryViews(), Tx: fakeTx{}, Photos: f.photos,
		Now: func() time.Time { return f.now },
	})

	return f
}

func (f *storyFixture) tell(t *testing.T, authorID uuid.UUID, text post.StoryText) *post.Story {
	t.Helper()

	story, err := f.svc.Tell(context.Background(), authorID, uuid.New(), text)
	if err != nil {
		t.Fatal(err)
	}

	return story
}

func (f *storyFixture) follow(t *testing.T, readerID, sourceID uuid.UUID) {
	t.Helper()

	if err := f.store.Sources().Add(context.Background(), readerID, sourceID, false); err != nil {
		t.Fatal(err)
	}
}

func storyIDs(stories []*post.Story) []uuid.UUID {
	ids := make([]uuid.UUID, len(stories))
	for i, s := range stories {
		ids[i] = s.ID()
	}

	return ids
}

func TestStoryService_Tell(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	f := newStoryFixture()

	story := f.tell(t, ann, "Утро")
	if len(f.photos.published) != 1 || f.photos.published[0].owner != ann || f.photos.published[0].photos[0] != story.PhotoID() || f.photos.outsideTx != 0 {
		t.Fatalf("the photo is kept at once, in the transaction: %+v", f.photos.published)
	}

	if _, err := f.svc.Tell(ctx, ann, uuid.Nil, ""); !errors.Is(err, post.ErrStoryWithoutPhoto) {
		t.Fatalf("a story without a photo: %v", err)
	}

	for range post.MaxStories - 1 {
		f.tell(t, ann, "")
	}
	if _, err := f.svc.Tell(ctx, ann, uuid.New(), ""); !errors.Is(err, post.ErrTooManyStories) {
		t.Fatalf("too many stories: %v", err)
	}

	// A day later the first are gone and there is room again.
	f.now = f.now.Add(post.StoryLifetime)
	f.tell(t, ann, "")
}

func TestStoryService_Feed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, cat, dan, eve := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := newStoryFixture()
	f.follow(t, ann, bob)
	f.follow(t, ann, cat)
	f.follow(t, ann, dan)

	bob1 := f.tell(t, bob, "1")
	f.now = f.now.Add(time.Minute)
	cat1 := f.tell(t, cat, "1")
	f.now = f.now.Add(time.Minute)
	bob2 := f.tell(t, bob, "2")
	f.now = f.now.Add(time.Minute)
	own := f.tell(t, ann, "своя")
	f.tell(t, eve, "не читает")
	dan1 := f.tell(t, dan, "1")

	// cat's story is viewed: bob, with the latest of the rest, goes first
	// after ann's own; dan is blacklisted.
	if err := f.svc.View(ctx, ann, cat1.ID()); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)

	page, err := f.svc.Feed(ctx, service.StoryAudience{Viewer: ann, Hidden: []uuid.UUID{dan}})
	if err != nil {
		t.Fatal(err)
	}

	want := []uuid.UUID{own.ID(), bob1.ID(), bob2.ID(), cat1.ID()}
	if got := storyIDs(page.Stories); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("the feed = %v, want %v", got, want)
	}
	if !page.Viewed[cat1.ID()] || page.Viewed[bob1.ID()] || page.Views[own.ID()] != 0 {
		t.Fatalf("viewed %v, views %v", page.Viewed, page.Views)
	}

	// Ignored, dan is gone too; not blacklisted, he is back.
	page, _ = f.svc.Feed(ctx, service.StoryAudience{Viewer: ann, Ignored: []uuid.UUID{bob}})
	if got := storyIDs(page.Stories); len(got) != 3 || got[1] != dan1.ID() {
		t.Fatalf("without bob the feed = %v", got)
	}

	// A day after, nothing is left.
	f.now = f.now.Add(post.StoryLifetime)
	if page, _ = f.svc.Feed(ctx, service.StoryAudience{Viewer: ann}); len(page.Stories) != 0 {
		t.Fatalf("the stories of yesterday: %d", len(page.Stories))
	}
}

func TestStoryService_Hidden(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	f := newStoryFixture()
	f.follow(t, ann, bob)
	story := f.tell(t, bob, "")

	// A closed page shows its stories only to those its owner follows.
	if err := f.store.Sources().Close(ctx, bob, false); err != nil {
		t.Fatal(err)
	}
	if page, _ := f.svc.User(ctx, service.StoryAudience{Viewer: ann}, bob); len(page.Stories) != 0 {
		t.Fatal("a closed page shows its stories")
	}
	if err := f.svc.View(ctx, ann, story.ID()); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("a story of a closed page viewed: %v", err)
	}
	f.follow(t, bob, ann)
	if page, _ := f.svc.User(ctx, service.StoryAudience{Viewer: ann}, bob); len(page.Stories) != 1 {
		t.Fatal("bob follows ann, she sees his stories")
	}

	// A deleted page shows none.
	if err := f.store.Holds().Hold(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if page, _ := f.svc.User(ctx, service.StoryAudience{Viewer: cat}, bob); len(page.Stories) != 0 {
		t.Fatal("a deleted page shows its stories")
	}
}

func TestStoryService_Views(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	f := newStoryFixture()
	story := f.tell(t, ann, "")

	for _, viewer := range []uuid.UUID{ann, bob, bob, cat} {
		if err := f.svc.View(ctx, viewer, story.ID()); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(time.Second)
	}

	// The author's own view is not counted, but she viewed it.
	page, err := f.svc.Feed(ctx, service.StoryAudience{Viewer: ann})
	if err != nil || page.Views[story.ID()] != 2 || !page.Viewed[story.ID()] {
		t.Fatalf("views %v, viewed %v: %v", page.Views, page.Viewed, err)
	}

	viewers, err := f.svc.Viewers(ctx, ann, story.ID(), nil, 1)
	if err != nil || viewers.Total != 2 || len(viewers.Views) != 1 || viewers.Views[0].ViewerID != cat || viewers.Next == nil {
		t.Fatalf("the first page of viewers = %+v: %v", viewers, err)
	}
	viewers, _ = f.svc.Viewers(ctx, ann, story.ID(), viewers.Next, 1)
	if len(viewers.Views) != 1 || viewers.Views[0].ViewerID != bob || viewers.Next != nil {
		t.Fatalf("the second page of viewers = %+v", viewers)
	}

	if _, err = f.svc.Viewers(ctx, bob, story.ID(), nil, 10); !errors.Is(err, post.ErrNotStoryAuthor) {
		t.Fatalf("bob sees the viewers: %v", err)
	}
}

func TestStoryService_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	f := newStoryFixture()
	story := f.tell(t, ann, "")

	if err := f.svc.Delete(ctx, bob, story.ID()); !errors.Is(err, post.ErrCannotDeleteStory) {
		t.Fatalf("bob deletes ann's story: %v", err)
	}
	if err := f.svc.Delete(ctx, ann, story.ID()); err != nil {
		t.Fatal(err)
	}
	if len(f.photos.deleted) != 1 || f.photos.deleted[0].photos[0] != story.PhotoID() || f.photos.outsideTx != 0 {
		t.Fatalf("the photo goes with the story: %+v", f.photos.deleted)
	}
	if err := f.svc.Delete(ctx, ann, story.ID()); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}

func TestStoryService_DeleteExpired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	f := newStoryFixture()
	old := f.tell(t, ann, "")
	f.now = f.now.Add(time.Hour)
	fresh := f.tell(t, ann, "")

	f.now = f.now.Add(post.StoryLifetime - time.Minute)
	deleted, err := f.svc.DeleteExpired(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteExpired = %d, %v", deleted, err)
	}
	if _, err = f.store.Stories().Get(ctx, old.ID()); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatal("the old story stays")
	}
	if _, err = f.store.Stories().Get(ctx, fresh.ID()); err != nil {
		t.Fatal("the fresh story is gone")
	}
	if len(f.photos.deleted) != 1 || f.photos.deleted[0].photos[0] != old.PhotoID() {
		t.Fatalf("the photos deleted: %+v", f.photos.deleted)
	}
}
