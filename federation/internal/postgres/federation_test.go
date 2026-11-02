package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestRepositories(t *testing.T) {
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

	accounts := postgres.NewAccountRepository(pool)
	followers := postgres.NewFollowerRepository(pool)
	remotes := postgres.NewRemoteRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	user := uuid.New()
	if _, err = accounts.Get(ctx, user); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of no account = %v", err)
	}

	account, err := fedi.NewAccount(user, fedi.KeyPair{Public: "pub", Private: "priv"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = accounts.Save(ctx, account); err != nil {
		t.Fatal(err)
	}

	// Saving again keeps the keys and changes only whether it is open.
	other, err := fedi.NewAccount(user, fedi.KeyPair{Public: "other", Private: "other"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = accounts.Save(ctx, other.Closed()); err != nil {
		t.Fatal(err)
	}

	got, err := accounts.Get(ctx, user)
	if err != nil || got.Open() || got.Keys().Private != "priv" || !got.CreatedAt().Equal(now) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	host := "https://" + uuid.NewString() + ".example"
	anya := remote(t, host+"/users/anya", host+"/inbox", now)
	boris := remote(t, host+"/users/boris", host+"/inbox", now)

	for _, r := range []*fedi.Remote{anya, boris} {
		if err = followers.Add(ctx, fedi.NewFollower(user, r, now)); err != nil {
			t.Fatal(err)
		}
	}
	if err = followers.Add(ctx, fedi.NewFollower(user, anya, now)); err != nil {
		t.Fatalf("Add twice: %v", err)
	}

	if n, err := followers.Count(ctx, user); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	inboxes, err := followers.Inboxes(ctx, user)
	if err != nil || len(inboxes) != 1 || inboxes[0] != fedi.URL(host+"/inbox") {
		t.Fatalf("Inboxes = %v, %v; want the shared inbox once", inboxes, err)
	}

	if err = followers.Remove(ctx, user, anya.ID()); err != nil {
		t.Fatal(err)
	}
	if err = followers.RemoveActor(ctx, boris.ID()); err != nil {
		t.Fatal(err)
	}
	if n, _ := followers.Count(ctx, user); n != 0 {
		t.Fatalf("Count after removing = %d", n)
	}

	if err = followers.Add(ctx, fedi.NewFollower(user, anya, now)); err != nil {
		t.Fatal(err)
	}
	if err = followers.RemoveAll(ctx, user); err != nil {
		t.Fatal(err)
	}
	if n, _ := followers.Count(ctx, user); n != 0 {
		t.Fatalf("Count after RemoveAll = %d", n)
	}

	if err = remotes.Save(ctx, anya); err != nil {
		t.Fatal(err)
	}
	if err = remotes.Save(ctx, anya); err != nil {
		t.Fatalf("Save twice: %v", err)
	}

	r, err := remotes.Get(ctx, anya.ID())
	if err != nil || r.DeliveryInbox() != fedi.URL(host+"/inbox") || r.PublicKey() != "PEM" || !r.FetchedAt().Equal(now) {
		t.Fatalf("Get = %+v, %v", r, err)
	}

	if err = remotes.Delete(ctx, anya.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = remotes.Get(ctx, anya.ID()); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of a deleted remote = %v", err)
	}
}

func remote(t *testing.T, id, shared string, now time.Time) *fedi.Remote {
	t.Helper()

	r, err := fedi.NewRemote(fedi.URL(id), fedi.RemoteDoc{ID: id, Inbox: id + "/inbox", SharedInbox: shared, KeyID: id + "#main-key", PublicKey: "PEM"}, now)
	if err != nil {
		t.Fatal(err)
	}

	return r
}
