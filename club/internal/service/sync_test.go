package service_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/service"
)

// toldRoster is a stretch of members told to post.
type toldRoster struct {
	club, after uuid.UUID
	ids         []uuid.UUID
	last        bool
}

type fakeRosters struct {
	told []toldRoster
	err  error
}

func (f *fakeRosters) Roster(_ context.Context, clubID, after uuid.UUID, ids []uuid.UUID, last bool) error {
	if f.err != nil {
		return f.err
	}

	f.told = append(f.told, toldRoster{clubID, after, slices.Clone(ids), last})

	return nil
}

// fakeBook is clubs with their members, in order; one of them has more
// members than a stretch holds.
type fakeBook struct {
	clubs   []uuid.UUID
	members map[uuid.UUID][]uuid.UUID
	locked  []uuid.UUID
}

func (f *fakeBook) ClubIDs(_ context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return aboveID(f.clubs, after, limit), nil
}

func (f *fakeBook) MemberIDs(_ context.Context, clubID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	return aboveID(f.members[clubID], after, limit), nil
}

func (f *fakeBook) LockClub(_ context.Context, clubID uuid.UUID) error {
	f.locked = append(f.locked, clubID)
	return nil
}

// aboveID are up to limit of the sorted ids above after.
func aboveID(ids []uuid.UUID, after uuid.UUID, limit int) []uuid.UUID {
	from := 0
	for from < len(ids) && bytes.Compare(ids[from][:], after[:]) <= 0 {
		from++
	}

	return ids[from:min(from+limit, len(ids))]
}

func sortedIDs(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}

	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	return ids
}

func TestReaderSyncService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clubs := sortedIDs(3)
	book := &fakeBook{clubs: clubs, members: map[uuid.UUID][]uuid.UUID{clubs[0]: sortedIDs(2), clubs[2]: sortedIDs(2000)}}
	rosters := &fakeRosters{}
	sync := service.NewReaderSyncService(book, inmem.TxManager{}, rosters)

	if n, err := sync.SyncAll(ctx); err != nil || n != 3 {
		t.Fatalf("SyncAll = %d, %v; want every club", n, err)
	}

	// Each club under its own lock, once; a club nobody is in is told
	// too: post may still keep a member who left unheard.
	if !slices.Equal(book.locked, clubs) {
		t.Fatalf("locked %v, want each of %v once", book.locked, clubs)
	}

	told := make(map[uuid.UUID][]toldRoster)
	for _, r := range rosters.told {
		told[r.club] = append(told[r.club], r)
	}

	for i, stretches := range []int{1, 1, 3} {
		got := told[clubs[i]]
		if len(got) != stretches {
			t.Fatalf("club %d told in %d stretches, want %d", i, len(got), stretches)
		}

		var all []uuid.UUID
		after := uuid.Nil
		for j, r := range got {
			if r.after != after || r.last != (j == stretches-1) {
				t.Fatalf("club %d stretch %d starts above %v (last %v), want above %v", i, j, r.after, r.last, after)
			}

			all = append(all, r.ids...)
			if len(r.ids) > 0 {
				after = r.ids[len(r.ids)-1]
			}
		}

		if !slices.Equal(all, book.members[clubs[i]]) {
			t.Fatalf("club %d: told %d members, want the %d there are", i, len(all), len(book.members[clubs[i]]))
		}
	}

	rosters.err = errors.New("the outbox is down")
	if _, err := sync.SyncAll(ctx); !errors.Is(err, rosters.err) {
		t.Fatalf("SyncAll with a failing outbox = %v", err)
	}
}
