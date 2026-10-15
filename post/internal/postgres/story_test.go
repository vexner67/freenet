package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestStoryRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := newPool(t)
	repo, views := postgres.NewStoryRepository(pool), postgres.NewStoryViewRepository(pool)
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	tell := func(authorID uuid.UUID, at time.Time) *post.Story {
		t.Helper()
		s, err := post.NewStory(authorID, uuid.New(), "Утро", at)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, s); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return s
	}

	old := tell(bob, now.Add(-post.StoryLifetime))
	first := tell(bob, now.Add(-time.Hour))
	second := tell(bob, now.Add(-time.Minute))
	own := tell(ann, now.Add(-2*time.Minute))
	stranger := tell(dan, now.Add(-time.Minute))

	if got, err := repo.Get(ctx, first.ID()); err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, first)
	}

	// ann reads bob and cat; dan she does not.
	feed := reader(t, []uuid.UUID{bob, cat})
	sources := postgres.NewSourceRepository(pool)
	viewer := feed.Reader
	mine := tell(viewer, now.Add(-time.Second))

	got, err := repo.Shown(ctx, uuid.Nil, service.StoryAudience{Viewer: viewer}, now)
	if err != nil || len(got) != 3 || got[0].ID() != first.ID() || got[1].ID() != second.ID() || got[2].ID() != mine.ID() {
		t.Fatalf("Shown in the feed = %v, %v", got, err)
	}
	if got, _ = repo.Shown(ctx, uuid.Nil, service.StoryAudience{Viewer: viewer, Ignored: []uuid.UUID{bob}}, now); len(got) != 1 {
		t.Fatalf("Shown without bob = %v", got)
	}
	if got, _ = repo.Shown(ctx, bob, service.StoryAudience{Viewer: viewer, Hidden: []uuid.UUID{bob}}, now); len(got) != 0 {
		t.Fatalf("Shown of a blacklisted bob = %v", got)
	}
	if got, _ = repo.Shown(ctx, dan, service.StoryAudience{}, now); len(got) != 1 || got[0].ID() != stranger.ID() {
		t.Fatalf("Shown of dan to a guest = %v", got)
	}

	// A closed page shows its stories only to those its owner follows.
	if err = sources.Close(ctx, dan, false); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Seen(ctx, stranger.ID(), viewer, now); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("Seen on a closed page: %v", err)
	}
	if _, err = repo.Seen(ctx, stranger.ID(), dan, now); err != nil {
		t.Fatalf("Seen by its author: %v", err)
	}
	if _, err = repo.Seen(ctx, old.ID(), viewer, now); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("Seen of yesterday: %v", err)
	}

	// The limit counts the live ones.
	for range post.MaxStories - 1 {
		tell(ann, now)
	}
	s, _ := post.NewStory(ann, uuid.New(), "", now)
	if err = repo.Save(ctx, s); !errors.Is(err, post.ErrTooManyStories) {
		t.Fatalf("Save over the limit: %v", err)
	}

	// Views: once each, the author's not counted, deleted pages not counted.
	for i, v := range []uuid.UUID{bob, viewer, viewer, cat, ann} {
		if err = views.View(ctx, own.ID(), v, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("View: %v", err)
		}
	}
	if err = views.View(ctx, uuid.New(), bob, now); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("View of no story: %v", err)
	}
	if err = postgres.NewHoldRepository(pool).Hold(ctx, cat); err != nil {
		t.Fatal(err)
	}

	counts, err := views.Count(ctx, []uuid.UUID{own.ID(), first.ID()})
	if err != nil || counts[own.ID()] != 2 || counts[first.ID()] != 0 {
		t.Fatalf("Count = %v, %v", counts, err)
	}
	viewed, err := views.Viewed(ctx, bob, []uuid.UUID{own.ID(), first.ID()})
	if err != nil || !viewed[own.ID()] || viewed[first.ID()] {
		t.Fatalf("Viewed = %v, %v", viewed, err)
	}

	page, err := views.Viewers(ctx, own.ID(), nil, 1)
	if err != nil || len(page) != 1 || page[0].ViewerID != viewer || !page[0].ViewedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("Viewers = %+v, %v", page, err)
	}
	page, err = views.Viewers(ctx, own.ID(), &service.ViewCursor{ViewedAt: page[0].ViewedAt, ViewerID: page[0].ViewerID}, 10)
	if err != nil || len(page) != 1 || page[0].ViewerID != bob {
		t.Fatalf("Viewers after = %+v, %v", page, err)
	}

	expired, err := repo.Expired(ctx, now, 1000)
	if err != nil || !containsStory(expired, old.ID()) || containsStory(expired, first.ID()) {
		t.Fatalf("Expired = %v, %v", expired, err)
	}

	if err = repo.Delete(ctx, own.ID()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Delete(ctx, own.ID()); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}

	// The purge takes the stories and the views.
	if err = views.View(ctx, first.ID(), viewer, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.DeleteByUser(ctx, viewer); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, mine.ID()); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatal("the purged user's story stays")
	}
	if viewed, _ = views.Viewed(ctx, viewer, []uuid.UUID{first.ID()}); viewed[first.ID()] {
		t.Fatal("the purged user's view stays")
	}
}

func containsStory(stories []*post.Story, id uuid.UUID) bool {
	for _, s := range stories {
		if s.ID() == id {
			return true
		}
	}

	return false
}
