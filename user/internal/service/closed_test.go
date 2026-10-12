package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/user"
)

// fakeWalls records the pages closed and opened, in order.
type fakeWalls struct {
	heard []string
	err   error
}

func (f *fakeWalls) PageClosed(_ context.Context, id uuid.UUID) error {
	f.heard = append(f.heard, "closed "+id.String())
	return f.err
}

func (f *fakeWalls) PageOpened(_ context.Context, id uuid.UUID) error {
	f.heard = append(f.heard, "opened "+id.String())
	return f.err
}

func TestClosedPage(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	walls := &fakeWalls{}
	w.users.TellingWalls(inmem.TxManager{}, walls)

	set := func(closed bool) error {
		t.Helper()

		u, err := w.users.GetByID(w.ctx, w.ann)
		if err != nil {
			t.Fatal(err)
		}

		privacy := u.Privacy()
		privacy.Closed = closed
		_, err = w.users.UpdatePrivacy(w.ctx, w.ann, privacy)

		return err
	}

	if _, err := w.users.UpdateProfile(w.ctx, w.ann, user.Profile{Name: "Анна", Nickname: mustNickname(t, "ann")}, ""); err != nil {
		t.Fatal(err)
	}

	// Ann follows bob; cat follows ann.
	if err := w.follows.Follow(w.ctx, w.ann, w.bob); err != nil {
		t.Fatal(err)
	}
	if err := w.follows.Follow(w.ctx, w.cat, w.ann); err != nil {
		t.Fatal(err)
	}

	// Saving the settings of an open page tells nobody.
	if err := set(false); err != nil || len(walls.heard) != 0 {
		t.Fatalf("an open page saved: %v, heard %v", err, walls.heard)
	}

	for range 2 {
		if err := set(true); err != nil {
			t.Fatalf("closing: %v", err)
		}
	}
	if len(walls.heard) != 1 || walls.heard[0] != "closed "+w.ann.String() {
		t.Fatalf("heard %v, want ann's page closed once", walls.heard)
	}

	tests := []struct {
		name   string
		viewer uuid.UUID
		sees   bool
	}{
		{"nobody", uuid.Nil, false},
		{"a follower", w.cat, false},
		{"whom ann follows", w.bob, true},
		{"ann", w.ann, true},
	}

	for _, tt := range tests {
		access, err := w.permissions.Access(w.ctx, tt.viewer, w.ann)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if access.Page != tt.sees || access.Photos != tt.sees || access.Notes != tt.sees {
			t.Errorf("%s: access = %+v, want %v", tt.name, access, tt.sees)
		}

		p, err := w.permissions.ProfileFor(w.ctx, tt.viewer, mustNickname(t, "ann"))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if p.PageHidden == tt.sees || !p.Closed {
			t.Errorf("%s: profile hidden %v, closed %v", tt.name, p.PageHidden, p.Closed)
		}

		if tt.viewer == uuid.Nil {
			continue
		}

		can, err := w.permissions.CanPostOnWall(w.ctx, tt.viewer, w.ann)
		if err != nil || can != tt.sees {
			t.Errorf("%s writes on the closed wall: %v, %v", tt.name, can, err)
		}
	}

	if err := set(false); err != nil || len(walls.heard) != 2 || walls.heard[1] != "opened "+w.ann.String() {
		t.Fatalf("opening: %v, heard %v", err, walls.heard)
	}

	// The page stays as it was when the word about its wall is not recorded.
	walls.err = errors.New("no outbox")
	if err := set(true); err == nil {
		t.Fatal("closing without the outbox succeeded")
	}
}

// A list of people does not know who looks: a closed page tells it no
// information and not that its owner is online.
func TestClosedPage_Listed(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	u, err := w.users.GetByID(w.ctx, w.ann)
	if err != nil {
		t.Fatal(err)
	}

	privacy := u.Privacy()
	privacy.Closed = true
	if _, err = w.users.UpdatePrivacy(w.ctx, w.ann, privacy); err != nil {
		t.Fatal(err)
	}

	profiles, err := w.users.GetPublicProfiles(w.ctx, []uuid.UUID{w.ann})
	if err != nil || len(profiles) != 1 {
		t.Fatalf("GetPublicProfiles: %v, %v", profiles, err)
	}
	if p := profiles[0]; !p.Closed || !p.InfoHidden || p.PageHidden || p.Online {
		t.Fatalf("listed = %+v", p)
	}
}
