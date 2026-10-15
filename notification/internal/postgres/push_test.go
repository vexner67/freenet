package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/postgres"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestSubscriptions(t *testing.T) {
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

	repo := postgres.NewSubscriptionRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	keys := push.Keys{P256DH: append([]byte{4}, make([]byte, 64)...), Auth: make([]byte, 16)}
	prefix := "https://push.example/" + uuid.NewString() + "/"

	save := func(userID uuid.UUID, name string) {
		t.Helper()

		s, err := push.NewSubscription(userID, push.Endpoint(prefix+name), keys, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	list := func(userID uuid.UUID) []*push.Subscription {
		t.Helper()

		subs, err := repo.List(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}

		return subs
	}

	for i := range push.MaxSubscriptions + 2 {
		save(ann, string(rune('a'+i)))
	}

	subs := list(ann)
	if len(subs) != push.MaxSubscriptions || subs[0].Endpoint() != push.Endpoint(prefix+"l") || subs[len(subs)-1].Endpoint() != push.Endpoint(prefix+"c") {
		t.Fatalf("ann keeps %d browsers", len(subs))
	}

	if got := subs[0].Keys(); len(got.P256DH) != 65 || len(got.Auth) != 16 {
		t.Fatalf("keys read back: %v", got)
	}

	// Bob takes a browser over; ann cannot delete it, he can.
	save(bob, "l")
	if len(list(ann)) != push.MaxSubscriptions-1 || len(list(bob)) != 1 {
		t.Fatal("the browser did not move to bob")
	}

	if err = repo.Delete(ctx, ann, push.Endpoint(prefix+"l")); err != nil || len(list(bob)) != 1 {
		t.Fatalf("ann deleted bob's browser: %v", err)
	}

	if err = repo.Delete(ctx, bob, push.Endpoint(prefix+"l")); err != nil || len(list(bob)) != 0 {
		t.Fatalf("bob kept his browser: %v", err)
	}

	if err = repo.Forget(ctx, push.Endpoint(prefix+"k")); err != nil || len(list(ann)) != push.MaxSubscriptions-2 {
		t.Fatalf("the gone browser is kept: %v", err)
	}

	if err = repo.DeleteByUser(ctx, ann); err != nil || len(list(ann)) != 0 {
		t.Fatalf("a purged user keeps browsers: %v", err)
	}
}
