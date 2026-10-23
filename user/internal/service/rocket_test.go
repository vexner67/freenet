package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestRocketService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	casinos := inmem.NewCasinoRepository()
	casino := service.NewCasinoService(w.users, casinos, w.users, inmem.TxManager{}, admins)
	// Every rocket explodes at ×2.13: a roll of 60% at a return of 85%.
	rockets := service.NewRocketService(w.users, casinos, inmem.NewRocketRepository(), w.users, inmem.TxManager{}, admins).
		WithRoll(func(n int64) (int64, error) {
			if n != user.RocketRolls {
				t.Fatalf("rolled below %d", n)
			}

			return n / 10 * 6, nil
		}).
		WithNow(func() time.Time { return now })

	// Only the administrators change the rocket.
	tuned, err := user.NewCasinoRocket(10, 100, 8500, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := rockets.SetRocket(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetRocket by ann: %v", err)
	}
	if err := rockets.SetRocket(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}

	if _, err := rockets.Launch(ctx, ann, 100, 0); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Launch without votes: %v", err)
	}
	if _, err := rockets.Launch(ctx, ann, 5, 0); !errors.Is(err, user.ErrRocketAmount) {
		t.Fatalf("Launch below the least: %v", err)
	}
	if _, err := rockets.Launch(ctx, ann, 100, 2000); !errors.Is(err, user.ErrRocketAuto) {
		t.Fatalf("Launch past the top: %v", err)
	}
	if _, err := w.users.Credit(ctx, ann, 1000, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	// By hand: the crash point is not told while it flies.
	flying, err := rockets.Launch(ctx, ann, 100, 0)
	if err != nil || flying.State != user.RocketFlying || flying.Balance != 900 || flying.Crash() != 0 {
		t.Fatalf("Launch = %+v, %v", flying, err)
	}

	id := flying.Round.ID()
	if _, err := rockets.Round(ctx, bob, id); !errors.Is(err, user.ErrRocketNotFound) {
		t.Fatalf("Round of bob: %v", err)
	}
	if _, err := rockets.Cash(ctx, bob, id); !errors.Is(err, user.ErrRocketNotFound) {
		t.Fatalf("Cash by bob: %v", err)
	}
	if _, err := rockets.Cash(ctx, ann, uuid.New()); !errors.Is(err, user.ErrRocketNotFound) {
		t.Fatalf("Cash of nothing: %v", err)
	}

	// ×2 at 5.8 s.
	now = now.Add(5777 * time.Millisecond)

	cashed, err := rockets.Cash(ctx, ann, id)
	if err != nil || cashed.State != user.RocketCashed || cashed.Round.Payout() != 200 || cashed.Balance != 1100 || cashed.Crash() != 213 {
		t.Fatalf("Cash = %+v, %v", cashed, err)
	}
	if _, err := rockets.Cash(ctx, ann, id); !errors.Is(err, user.ErrRocketOver) {
		t.Fatalf("Cash twice: %v", err)
	}
	if read, err := rockets.Round(ctx, ann, id); err != nil || read.State != user.RocketCashed || read.Round.Cashed() != 200 {
		t.Fatalf("Round = %+v, %v", read, err)
	}

	// Too late: nothing comes back.
	late, err := rockets.Launch(ctx, ann, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)

	if read, err := rockets.Round(ctx, ann, late.Round.ID()); err != nil || read.State != user.RocketCrashed || read.Crash() != 213 {
		t.Fatalf("crashed Round = %+v, %v", read, err)
	}
	if lost, err := rockets.Cash(ctx, ann, late.Round.ID()); err != nil || lost.State != user.RocketCrashed || lost.Balance != 1000 {
		t.Fatalf("late Cash = %+v, %v", lost, err)
	}

	// By itself: paid at the launch at ×2, lost at ×3.
	if won, err := rockets.Launch(ctx, ann, 50, 200); err != nil || won.State != user.RocketCashed || won.Balance != 1050 || won.Crash() != 213 {
		t.Fatalf("auto ×2 = %+v, %v", won, err)
	}
	if lost, err := rockets.Launch(ctx, ann, 50, 300); err != nil || lost.State != user.RocketCrashed || lost.Balance != 1000 {
		t.Fatalf("auto ×3 = %+v, %v", lost, err)
	}
	if got := w.votes(t, ann); got != 1000 {
		t.Fatalf("votes = %d; want 1000", got)
	}

	settings, err := casino.Settings(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Rocket != tuned || settings.Stats != (user.CasinoStats{Rockets: 4, RocketSpent: 300, RocketWon: 300}) {
		t.Fatalf("Settings = %+v", settings)
	}
}
