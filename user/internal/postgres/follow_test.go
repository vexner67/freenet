package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

// The follows as the news of post need them: by ID, in stretches.
func TestFollowRepository_Book(t *testing.T) {
	repo, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ids := make([]uuid.UUID, 4)
	for i := range ids {
		u, err := user.New(user.Email(unique("fb") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		ids[i] = u.ID()
	}

	reader, followees := ids[0], ids[1:]
	slices.SortFunc(followees, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	for _, followee := range followees {
		if err := follows.Follow(ctx, reader, followee); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := follows.Followees(ctx, reader, uuid.Nil, 2); err != nil || !slices.Equal(got, followees[:2]) {
		t.Fatalf("Followees = %v, %v; want the first two of %v", got, err, followees)
	}
	if got, err := follows.Followees(ctx, reader, followees[1], 2); err != nil || !slices.Equal(got, followees[2:]) {
		t.Fatalf("Followees above the second = %v, %v", got, err)
	}
	if got, err := follows.Followees(ctx, followees[0], uuid.Nil, 2); err != nil || len(got) != 0 {
		t.Fatalf("Followees of one who follows nobody = %v, %v", got, err)
	}

	// The users after the one before the reader start with the reader.
	if reader[15] == 0 {
		t.Skip("an ID ending in zero has no neighbour to start above")
	}

	before := reader
	before[15]--

	got, err := follows.Readers(ctx, before, 1)
	if err != nil || len(got) != 1 || got[0] != reader {
		t.Fatalf("Readers = %v, %v; want %v", got, err, reader)
	}
	if got, err = follows.Readers(ctx, reader, 1000); err != nil || slices.Contains(got, reader) || !slices.IsSortedFunc(got, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }) {
		t.Fatalf("Readers above the reader = %v, %v", got, err)
	}
}

// The lock of a reader is held to the end of the transaction: whoever
// tells of the same reader's follows meanwhile waits.
func TestFollowRepository_LockReader(t *testing.T) {
	_, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reader, other := uuid.New(), uuid.New()

	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := follows.LockReader(ctx, reader); err != nil {
			return err
		}

		// Another reader is nobody's business.
		if err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
			return follows.LockReader(ctx, other)
		}); err != nil {
			t.Fatalf("the lock of another reader: %v", err)
		}

		waiting, stop := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer stop()

		err := tx.WithinTx(waiting, func(ctx context.Context) error {
			return follows.LockReader(ctx, reader)
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("the lock taken twice = %v, want it to wait", err)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Released with the transaction.
	if err = tx.WithinTx(ctx, func(ctx context.Context) error {
		return follows.LockReader(ctx, reader)
	}); err != nil {
		t.Fatalf("the lock after the transaction: %v", err)
	}
}
