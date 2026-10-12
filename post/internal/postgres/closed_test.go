package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestSourceRepository_ClosedWalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := newPool(t)
	posts, sources := postgres.NewPostRepository(pool), postgres.NewSourceRepository(pool)
	ann, bob, cat, club := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	for _, wall := range []uuid.UUID{ann, club} {
		p, err := post.New(wall, ann, post.RestoreContent("x", nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Ann follows bob; cat follows ann and is in the club.
	for _, add := range []struct {
		reader, source uuid.UUID
		club           bool
	}{{ann, bob, false}, {cat, ann, false}, {cat, club, true}} {
		if err := sources.Add(ctx, add.reader, add.source, add.club); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	closed := func(viewer uuid.UUID) map[uuid.UUID]bool {
		t.Helper()

		got, err := sources.ClosedTo(ctx, viewer, []uuid.UUID{ann, club, bob})
		if err != nil {
			t.Fatalf("ClosedTo: %v", err)
		}

		return got
	}

	if got := closed(uuid.Nil); len(got) != 0 {
		t.Fatalf("open walls are closed to nobody: %v", got)
	}

	// Closing twice is closing once.
	for range 2 {
		if err := sources.Close(ctx, ann, false); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	if err := sources.Close(ctx, club, true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tests := []struct {
		name      string
		viewer    uuid.UUID
		ann, club bool
	}{
		{"nobody", uuid.Nil, true, true},
		{"the owner", ann, false, true},
		{"whom the owner follows", bob, false, true},
		{"a follower and member", cat, true, false},
	}

	for _, tt := range tests {
		got := closed(tt.viewer)
		if got[ann] != tt.ann || got[club] != tt.club || got[bob] {
			t.Errorf("%s: closed = %v, want ann %v, club %v", tt.name, got, tt.ann, tt.club)
		}
	}

	feed := func(reader uuid.UUID, everyone bool) int {
		t.Helper()

		page, err := posts.ListFeed(ctx, service.FeedSources{Reader: reader, Everyone: everyone}, uuid.Nil, 100)
		if err != nil {
			t.Fatalf("ListFeed: %v", err)
		}

		n := 0
		for _, p := range page {
			if p.WallID() == ann || p.WallID() == club {
				n++
			}
		}

		return n
	}

	// Cat reads the club she is in and not the page that does not follow her.
	if got := feed(cat, false); got != 1 {
		t.Errorf("cat reads %d posts, want the club's", got)
	}
	if got := feed(cat, true); got != 1 {
		t.Errorf("everyone's news show cat %d posts, want the club's", got)
	}
	if got := feed(bob, true); got != 1 {
		t.Errorf("everyone's news show bob %d posts, want ann's", got)
	}
	if got := feed(ann, false); got != 1 {
		t.Errorf("ann reads %d posts, want her own", got)
	}

	if err := sources.Open(ctx, ann); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := closed(cat); got[ann] {
		t.Error("the opened wall is still closed")
	}

	// An erased club is closed no more.
	if err := sources.Forget(ctx, club); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if got := closed(uuid.Nil); len(got) != 0 {
		t.Errorf("the erased wall is still closed: %v", got)
	}
}
