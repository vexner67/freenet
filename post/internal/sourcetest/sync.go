// Package sourcetest checks that a storage of feed sources takes the
// stretches the user and club services tell (SyncSources): the same test
// runs over PostgreSQL and the in-memory store.
package sourcetest

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/service"
)

// RunSync tells stretches of follows and of members and looks at what the
// storage keeps afterwards.
func RunSync(t *testing.T, sources service.SourceRepository) {
	t.Helper()

	ctx := context.Background()
	reader, club := uuid.New(), uuid.New()
	people, members := ascending(5), ascending(3)

	// A closed wall tells who is kept: a person's is read by those the
	// person follows, a community's by its members.
	if err := sources.Close(ctx, reader, false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := sources.Close(ctx, club, true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	follows := func(want ...int) {
		t.Helper()

		for i, p := range people {
			closed, err := sources.ClosedTo(ctx, p, []uuid.UUID{reader})
			if err != nil {
				t.Fatalf("ClosedTo: %v", err)
			}
			if followed := !closed[reader]; followed != slices.Contains(want, i) {
				t.Fatalf("person %d followed = %v, want those of %v", i, followed, want)
			}
		}
	}
	in := func(userID uuid.UUID) bool {
		t.Helper()

		closed, err := sources.ClosedTo(ctx, userID, []uuid.UUID{club})
		if err != nil {
			t.Fatalf("ClosedTo: %v", err)
		}

		return !closed[club]
	}
	sync := func(s service.SourceSync) {
		t.Helper()

		// Told twice, a stretch is the same stretch.
		for range 2 {
			if err := sources.Sync(ctx, s); err != nil {
				t.Fatalf("Sync: %v", err)
			}
		}
	}

	// The word of following 1 was lost, and of no longer following 2 and 4.
	for _, i := range []int{0, 2, 4} {
		if err := sources.Add(ctx, reader, people[i], false); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := sources.Add(ctx, reader, club, true); err != nil {
		t.Fatalf("Add: %v", err)
	}
	follows(0, 2, 4)

	// The first stretch says nothing of what lies beyond it.
	sync(service.SourceSync{Owner: reader, IDs: []uuid.UUID{people[0], people[1]}})
	follows(0, 1, 2, 4)

	sync(service.SourceSync{Owner: reader, After: people[1], IDs: []uuid.UUID{people[3]}, Last: true})
	follows(0, 1, 3)

	// The follows of a reader leave their communities alone.
	if !in(reader) {
		t.Fatal("the reader is out of the community after a stretch of follows")
	}

	// The same for the members: 0 and 2 left, 1 joined, all unheard.
	for _, i := range []int{0, 2} {
		if err := sources.Add(ctx, members[i], club, true); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := sources.Add(ctx, people[0], members[0], false); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sync(service.SourceSync{Owner: club, Club: true, IDs: []uuid.UUID{members[1]}, Last: true})

	for i, want := range []bool{false, true, false} {
		if got := in(members[i]); got != want {
			t.Fatalf("member %d in the community = %v, want %v", i, got, want)
		}
	}
	if in(reader) {
		t.Fatal("the reader is still in the community it has left")
	}

	// And the members of a community leave the follows alone.
	follows(0, 1, 3)

	if err := sources.Close(ctx, people[0], false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed, err := sources.ClosedTo(ctx, members[0], []uuid.UUID{people[0]}); err != nil || closed[people[0]] {
		t.Fatalf("the follow of a member who left = %v, %v", !closed[people[0]], err)
	}

	// Nobody followed at all.
	sync(service.SourceSync{Owner: reader, Last: true})
	follows()
}

// ascending are n IDs in ascending order.
func ascending(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}

	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	})

	return ids
}
