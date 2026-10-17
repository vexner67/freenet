package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/postgres"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// The memberships as the news of post need them: by ID, in stretches. The
// same over PostgreSQL and in memory.
func TestBook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type storage struct {
		clubs   service.ClubRepository
		members service.MemberRepository
		book    service.MemberBook
	}

	store := inmem.NewStore()
	storages := map[string]storage{"inmem": {store.Clubs(), store.Members(), store.Book()}}

	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		pool, err := pgpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)

		storages["postgres"] = storage{postgres.NewClubRepository(pool), postgres.NewMemberRepository(pool), postgres.NewBookRepository(pool)}
	}

	for name, s := range storages {
		t.Run(name, func(t *testing.T) {
			c, err := club.New("Любители котов", "", time.Now())
			if err != nil {
				t.Fatal(err)
			}

			members := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
			if err = s.clubs.Create(ctx, c, members[0]); err != nil {
				t.Fatalf("Create: %v", err)
			}
			for _, m := range members[1:] {
				if err = s.members.Join(ctx, c.ID(), m); err != nil {
					t.Fatalf("Join: %v", err)
				}
			}

			slices.SortFunc(members, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

			if got, err := s.book.MemberIDs(ctx, c.ID(), uuid.Nil, 2); err != nil || !slices.Equal(got, members[:2]) {
				t.Fatalf("MemberIDs = %v, %v; want the first two of %v", got, err, members)
			}
			if got, err := s.book.MemberIDs(ctx, c.ID(), members[1], 2); err != nil || !slices.Equal(got, members[2:]) {
				t.Fatalf("MemberIDs above the second = %v, %v", got, err)
			}
			if got, err := s.book.MemberIDs(ctx, uuid.New(), uuid.Nil, 2); err != nil || len(got) != 0 {
				t.Fatalf("MemberIDs of no club = %v, %v", got, err)
			}

			// Other runs share the database: walk the clubs to this one.
			found, after := false, uuid.Nil
			for !found {
				ids, err := s.book.ClubIDs(ctx, after, 100)
				if err != nil {
					t.Fatalf("ClubIDs: %v", err)
				}
				if len(ids) == 0 {
					break
				}
				if bytes.Compare(ids[0][:], after[:]) <= 0 || !slices.IsSortedFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }) {
					t.Fatalf("ClubIDs above %v = %v, want them ascending", after, ids)
				}

				found, after = slices.Contains(ids, c.ID()), ids[len(ids)-1]
			}
			if !found {
				t.Fatal("ClubIDs never listed the club")
			}
		})
	}
}

// The lock of a club is held to the end of the transaction: whoever tells
// of the same club's members meanwhile waits.
func TestBookRepository_LockClub(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	book, tx := postgres.NewBookRepository(pool), pgpool.NewTxManager(pool)
	cats, dogs := uuid.New(), uuid.New()

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := book.LockClub(ctx, cats); err != nil {
			return err
		}

		// Another club is nobody's business.
		if err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
			return book.LockClub(ctx, dogs)
		}); err != nil {
			t.Fatalf("the lock of another club: %v", err)
		}

		waiting, stop := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer stop()

		err := tx.WithinTx(waiting, func(ctx context.Context) error {
			return book.LockClub(ctx, cats)
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
		return book.LockClub(ctx, cats)
	}); err != nil {
		t.Fatalf("the lock after the transaction: %v", err)
	}
}
