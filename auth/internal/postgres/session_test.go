package postgres_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/postgres"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func newRepo(t *testing.T) *postgres.SessionRepository {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return postgres.NewSessionRepository(pool)
}

func TestSessionRepository(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	signedIn := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	phone := session.NewDevice("203.0.113.7", "Mozilla/5.0 (phone)")

	sess, err := session.New(uuid.New(), uuid.New(), "hash-1", phone, signedIn)
	if err != nil {
		t.Fatal(err)
	}

	if err = repo.Save(ctx, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.Get(ctx, sess.ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID() != sess.UserID() || got.RefreshTokenHash() != "hash-1" {
		t.Fatalf("got %+v", got)
	}
	if got.Device() != phone || !got.UsedAt().Equal(signedIn) || !got.CreatedAt().Equal(signedIn) {
		t.Fatalf("a new session = %v, used %v, created %v", got.Device(), got.UsedAt(), got.CreatedAt())
	}

	laptop, refreshed := session.NewDevice("2001:db8::1", "Mozilla/5.0 (laptop)"), signedIn.Add(time.Hour)
	if err = repo.Rotate(ctx, sess.Rotated("hash-2", laptop, refreshed), "hash-1"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err = repo.Rotate(ctx, sess.Rotated("hash-3", phone, refreshed.Add(time.Hour)), "hash-1"); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("rotate with a stale hash: got %v, want ErrSessionNotFound", err)
	}

	got, err = repo.Get(ctx, sess.ID())
	if err != nil || got.RefreshTokenHash() != "hash-2" {
		t.Fatalf("after rotate: %v, %v", got, err)
	}
	if got.Device() != laptop || !got.UsedAt().Equal(refreshed) || !got.CreatedAt().Equal(signedIn) {
		t.Fatalf("a rotated session = %v, used %v, created %v", got.Device(), got.UsedAt(), got.CreatedAt())
	}

	if err = repo.Delete(ctx, sess.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = repo.Delete(ctx, sess.ID()); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("second Delete: got %v, want ErrSessionNotFound", err)
	}
	if _, err = repo.Get(ctx, sess.ID()); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrSessionNotFound", err)
	}
}

// TestSessionRepository_Owned: the sessions as their owner manages them.
func TestSessionRepository_Owned(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	ann, bob := uuid.New(), uuid.New()
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	// ann's sessions, each used an hour after the one before; and bob's.
	var anns []uuid.UUID
	for i := range 4 {
		s, err := session.New(uuid.New(), ann, "hash", session.NewDevice("203.0.113.7", "browser"), start.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, s); err != nil {
			t.Fatal(err)
		}
		anns = append(anns, s.ID())
	}
	bobs, err := session.New(uuid.New(), bob, "hash", session.Device{}, start)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, bobs); err != nil {
		t.Fatal(err)
	}

	ids := func(usedAfter time.Time, limit int) []uuid.UUID {
		t.Helper()

		sessions, err := repo.ListByUser(ctx, ann, usedAfter, limit)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}

		var out []uuid.UUID
		for _, s := range sessions {
			out = append(out, s.ID())
		}

		return out
	}

	if got, want := ids(start.Add(-time.Hour), 10), []uuid.UUID{anns[3], anns[2], anns[1], anns[0]}; !slices.Equal(got, want) {
		t.Fatalf("ListByUser = %v, want the latest used first %v", got, want)
	}
	if got, want := ids(start, 10), []uuid.UUID{anns[3], anns[2], anns[1]}; !slices.Equal(got, want) {
		t.Fatalf("ListByUser of those used after the first = %v, want %v", got, want)
	}
	if got, want := ids(start.Add(-time.Hour), 2), []uuid.UUID{anns[3], anns[2]}; !slices.Equal(got, want) {
		t.Fatalf("ListByUser of two = %v, want %v", got, want)
	}

	if err = repo.DeleteOwned(ctx, ann, bobs.ID()); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("ann ended bob's session: %v", err)
	}
	if err = repo.DeleteOwned(ctx, ann, anns[0]); err != nil {
		t.Fatalf("DeleteOwned: %v", err)
	}
	if err = repo.DeleteOwned(ctx, ann, anns[0]); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("second DeleteOwned: %v", err)
	}

	if n, err := repo.DeleteOthers(ctx, ann, anns[3]); err != nil || n != 2 {
		t.Fatalf("DeleteOthers = %d, %v; want the two left besides the kept", n, err)
	}
	if got, want := ids(start.Add(-time.Hour), 10), []uuid.UUID{anns[3]}; !slices.Equal(got, want) {
		t.Fatalf("after DeleteOthers = %v, want %v", got, want)
	}
	if _, err = repo.Get(ctx, bobs.ID()); err != nil {
		t.Fatalf("bob's session after ann's DeleteOthers: %v", err)
	}
}
