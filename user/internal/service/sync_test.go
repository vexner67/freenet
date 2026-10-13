package service_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// toldStretch is a stretch of follows told to post.
type toldStretch struct {
	reader, after uuid.UUID
	ids           []uuid.UUID
	last          bool
}

type fakeSources struct {
	told []toldStretch
	err  error
}

func (f *fakeSources) Following(_ context.Context, readerID, after uuid.UUID, ids []uuid.UUID, last bool) error {
	if f.err != nil {
		return f.err
	}

	f.told = append(f.told, toldStretch{readerID, after, slices.Clone(ids), last})

	return nil
}

func TestSourceSyncService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)

	newUser := func(name string) uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(name + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = users.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	ann, bob, cat := newUser("ann"), newUser("bob"), newUser("cat")
	for _, followee := range []uuid.UUID{bob, cat} {
		if err := follows.Follow(ctx, ann, followee); err != nil {
			t.Fatal(err)
		}
	}
	if err := follows.Follow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}

	sources := &fakeSources{}
	sync := service.NewSourceSyncService(follows, inmem.TxManager{}, sources)

	if n, err := sync.SyncAll(ctx); err != nil || n != 3 {
		t.Fatalf("SyncAll = %d, %v; want every user", n, err)
	}

	// Everybody is told, those who follow nobody too: post may still
	// keep a follow of theirs that ended unheard.
	want := map[uuid.UUID][]uuid.UUID{ann: {bob, cat}, bob: {ann}, cat: nil}
	if len(sources.told) != len(want) {
		t.Fatalf("told %+v, want a stretch for each of %d", sources.told, len(want))
	}

	for i, told := range sources.told {
		ids := want[told.reader]
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

		if told.after != uuid.Nil || !told.last || !slices.Equal(told.ids, ids) {
			t.Fatalf("told %+v, want all of %v at once", told, ids)
		}
		if i > 0 && bytes.Compare(sources.told[i-1].reader[:], told.reader[:]) >= 0 {
			t.Fatalf("readers out of order: %+v", sources.told)
		}
	}

	sources.err = errors.New("the outbox is down")
	if _, err := sync.SyncAll(ctx); !errors.Is(err, sources.err) {
		t.Fatalf("SyncAll with a failing outbox = %v", err)
	}
}

// fakeBook is a reader who follows many: more than a stretch holds.
type fakeBook struct {
	reader    uuid.UUID
	followees []uuid.UUID
	locked    int
}

func (f *fakeBook) Readers(_ context.Context, after uuid.UUID, _ int) ([]uuid.UUID, error) {
	if after != uuid.Nil {
		return nil, nil
	}

	return []uuid.UUID{f.reader}, nil
}

func (f *fakeBook) Followees(_ context.Context, _, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	from, _ := slices.BinarySearchFunc(f.followees, after, func(id, target uuid.UUID) int {
		if c := bytes.Compare(id[:], target[:]); c != 0 {
			return c
		}
		// The stretch is above after.
		return -1
	})

	return f.followees[from:min(from+limit, len(f.followees))], nil
}

func (f *fakeBook) LockReader(context.Context, uuid.UUID) error {
	f.locked++
	return nil
}

func TestSourceSyncService_Stretches(t *testing.T) {
	t.Parallel()

	book := &fakeBook{reader: uuid.New(), followees: make([]uuid.UUID, 2000)}
	for i := range book.followees {
		book.followees[i] = uuid.New()
	}
	slices.SortFunc(book.followees, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	sources := &fakeSources{}
	if _, err := service.NewSourceSyncService(book, inmem.TxManager{}, sources).SyncAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Two full stretches and the end, each starting where the one before
	// stopped, under one lock.
	if len(sources.told) != 3 || book.locked != 1 {
		t.Fatalf("%d stretches under %d locks, want 3 under one", len(sources.told), book.locked)
	}

	var all []uuid.UUID
	after := uuid.Nil
	for i, told := range sources.told {
		if told.after != after || told.last != (i == 2) {
			t.Fatalf("stretch %d starts above %v (last %v), want above %v", i, told.after, told.last, after)
		}

		all = append(all, told.ids...)
		if len(told.ids) > 0 {
			after = told.ids[len(told.ids)-1]
		}
	}

	if !slices.Equal(all, book.followees) {
		t.Fatalf("told %d follows, want the %d there are", len(all), len(book.followees))
	}
}
