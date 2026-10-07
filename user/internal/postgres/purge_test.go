package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/purgeclient"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// A page due to be erased goes with its follows and blocks, and the erasure
// is recorded for each service in the same transaction.
func TestPurge(t *testing.T) {
	repo, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)
	blocks := postgres.NewBlockRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("pu") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}

	ann, bob := newUser(), newUser()
	if err := follows.Follow(ctx, bob.ID(), ann.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := blocks.Block(ctx, ann.ID(), bob.ID()); err != nil {
		t.Fatal(err)
	}

	if err := ann.Deactivate("", time.Now().Add(-time.Hour), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, ann); err != nil {
		t.Fatal(err)
	}

	streams := []string{"notification", purgeclient.PostStream, purgeclient.MediaStream, purgeclient.MessageStream}
	recorders := make([]purgeclient.Recorder, len(streams))
	for i, s := range streams {
		recorders[i] = outbox.NewPostgres(pool, s)
	}

	purge := service.NewPurgeService(repo, pgpool.NewTxManager(pool), purgeclient.NewOutbox(recorders...))

	erased, err := purge.PurgeDue(ctx)
	if err != nil || erased < 1 {
		t.Fatalf("PurgeDue = %d, %v", erased, err)
	}

	if _, err = repo.GetByID(ctx, ann.ID()); err == nil {
		t.Fatal("ann is still there")
	}
	if _, err = repo.GetByID(ctx, bob.ID()); err != nil {
		t.Fatalf("bob: %v", err)
	}
	if followers, following, _ := follows.Counts(ctx, bob.ID()); followers != 0 || following != 0 {
		t.Fatalf("bob's follows = %d, %d; want ann's gone", followers, following)
	}
	if byAnn, byBob, _ := blocks.Blocks(ctx, ann.ID(), bob.ID()); byAnn || byBob {
		t.Fatal("ann's block is still there")
	}

	annID := ann.ID()

	var recorded int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE kind = $1 AND payload = $2`, purgeclient.Kind, annID[:]).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != len(streams) {
		t.Fatalf("recorded the erasure %d times, want once per service", recorded)
	}

}
