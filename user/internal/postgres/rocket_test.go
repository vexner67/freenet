package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestRocketRepository(t *testing.T) {
	repo, pool := newRepo(t)
	casinos := postgres.NewCasinoRepository(pool)
	rockets := postgres.NewRocketRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it as the default rocket; the test leaves it so.
	t.Cleanup(func() {
		if err := casinos.SetRocket(context.Background(), user.DefaultCasinoRocket()); err != nil {
			t.Error(err)
		}
	})

	if got, err := casinos.Rocket(ctx); err != nil || got != user.DefaultCasinoRocket() {
		t.Fatalf("Rocket = %+v, %v; want the default", got, err)
	}

	tuned, err := user.NewCasinoRocket(5, 50, 8000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.SetRocket(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := casinos.Rocket(ctx); err != nil || got != tuned {
		t.Fatalf("Rocket = %+v, %v; want the tuned one", got, err)
	}

	before, err := casinos.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.CountRocket(ctx, 1, 40, 0); err != nil {
		t.Fatal(err)
	}
	if err = casinos.CountRocket(ctx, 0, 0, 90); err != nil {
		t.Fatal(err)
	}
	after, err := casinos.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Rocket != tuned || after.Stats.Rockets != before.Stats.Rockets+1 ||
		after.Stats.RocketSpent != before.Stats.RocketSpent+40 || after.Stats.RocketWon != before.Stats.RocketWon+90 {
		t.Fatalf("Settings = %+v; were %+v", after, before)
	}

	ann, err := user.New(user.Email(unique("rocket") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	flying, err := tuned.Launch(uuid.New(), ann.ID(), 40, 0, 300, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = rockets.Save(ctx, flying); err != nil {
		t.Fatal(err)
	}
	if got, err := rockets.Get(ctx, flying.ID()); err != nil || *got != *flying {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, flying)
	}

	cashed, err := flying.Cash(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := rockets.Lock(ctx, flying.ID()); err != nil {
			return err
		}

		return rockets.SetCashed(ctx, cashed)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rockets.Get(ctx, flying.ID()); err != nil || got.Cashed() != 112 || got.Payout() != 44 {
		t.Fatalf("Get after SetCashed = %+v, %v", got, err)
	}

	auto, err := tuned.Launch(uuid.New(), ann.ID(), 40, 200, 300, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = rockets.Save(ctx, auto); err != nil {
		t.Fatal(err)
	}
	if got, err := rockets.Get(ctx, auto.ID()); err != nil || *got != *auto {
		t.Fatalf("Get auto = %+v, %v; want %+v", got, err, auto)
	}

	if err = rockets.DeleteBefore(ctx, ann.ID(), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := rockets.Get(ctx, auto.ID()); !errors.Is(err, user.ErrRocketNotFound) {
		t.Fatalf("Get of an old round: %v", err)
	}
	if _, err := rockets.Get(ctx, flying.ID()); err != nil {
		t.Fatalf("Get of a new round: %v", err)
	}
	if err := rockets.SetCashed(ctx, auto); !errors.Is(err, user.ErrRocketNotFound) {
		t.Fatalf("SetCashed of nothing: %v", err)
	}
}
