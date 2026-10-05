package postgres_test

import (
	"context"
	"errors"
	"os"
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

	sess, err := session.New(uuid.New(), uuid.New(), "hash-1")
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

	if err = repo.Rotate(ctx, sess.ID(), "hash-1", "hash-2"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err = repo.Rotate(ctx, sess.ID(), "hash-1", "hash-3"); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("rotate with a stale hash: got %v, want ErrSessionNotFound", err)
	}

	got, err = repo.Get(ctx, sess.ID())
	if err != nil || got.RefreshTokenHash() != "hash-2" {
		t.Fatalf("after rotate: %v, %v", got, err)
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
