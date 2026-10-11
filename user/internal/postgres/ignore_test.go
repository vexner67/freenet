package postgres_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestIgnoreRepository(t *testing.T) {
	repo, pool := newRepo(t)
	ignores := postgres.NewIgnoreRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("ig") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	ann, bob := newUser(), newUser()
	// A community: the user service has no row of it.
	club := uuid.New()

	for _, source := range []uuid.UUID{bob, club, bob} {
		if ok, err := ignores.Ignore(ctx, ann, source, 2); err != nil || !ok {
			t.Fatalf("Ignore = %v, %v", ok, err)
		}
	}

	if got, err := ignores.Ignored(ctx, ann, 10); err != nil || !slices.Equal(got, []uuid.UUID{club, bob}) {
		t.Fatalf("Ignored = %v, %v; want the community, then bob", got, err)
	}
	if got, err := ignores.Ignored(ctx, ann, 1); err != nil || !slices.Equal(got, []uuid.UUID{club}) {
		t.Fatalf("Ignored, one = %v, %v", got, err)
	}

	if ok, err := ignores.Ignore(ctx, ann, uuid.New(), 2); err != nil || ok {
		t.Fatalf("over the limit = %v, %v", ok, err)
	}
	if ok, err := ignores.Ignore(ctx, ann, club, 2); err != nil || !ok {
		t.Fatalf("ignored already, at the limit = %v, %v", ok, err)
	}

	if _, err := ignores.Ignore(ctx, uuid.New(), bob, 2); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("nobody ignores: %v", err)
	}

	if err := ignores.Unignore(ctx, ann, []uuid.UUID{bob, uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ignores.Ignored(ctx, ann, 10); !slices.Equal(got, []uuid.UUID{club}) {
		t.Fatalf("Ignored after Unignore = %v", got)
	}

	// The ignores of a deleted user go with the user.
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ann); err != nil {
		t.Fatal(err)
	}

	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM feed_ignores WHERE user_id = $1`, ann).Scan(&left); err != nil || left != 0 {
		t.Fatalf("ignores of a deleted user = %d, %v", left, err)
	}
}

// Sources ignored at once never take more places than there are.
func TestIgnoreRepository_ConcurrentLimit(t *testing.T) {
	repo, pool := newRepo(t)
	ignores := postgres.NewIgnoreRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	u, err := user.New(user.Email(unique("ig") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	const limit, attempts = 3, 12

	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := ignores.Ignore(ctx, u.ID(), uuid.New(), limit); err != nil {
				t.Errorf("Ignore: %v", err)
			}
		}()
	}

	wg.Wait()

	got, err := ignores.Ignored(ctx, u.ID(), attempts)
	if err != nil || len(got) != limit {
		t.Fatalf("ignored %d sources, %v; want %d", len(got), err, limit)
	}
}
