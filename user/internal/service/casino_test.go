package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCasinoService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// The roll is the next of these.
	rolls := []int64{0, 3}
	casino := service.NewCasinoService(w.users, inmem.NewCasinoRepository(), w.users, inmem.TxManager{}, admins).
		WithRoll(func(n int64) (int64, error) {
			if n != 4 {
				t.Fatalf("rolled below %d", n)
			}
			roll := rolls[0]
			rolls = rolls[1:]

			return roll, nil
		})

	if c, err := casino.Casino(ctx); err != nil || c.Price() != user.DefaultCasino().Price() {
		t.Fatalf("Casino = %+v, %v", c, err)
	}

	// Only the administrators read and tune the odds.
	tuned, err := user.NewCasino(10, []user.CasinoPrize{{Votes: 0, Weight: 3}, {Votes: 25, Weight: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := casino.SetCasino(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetCasino by ann: %v", err)
	}
	if _, err := casino.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	if err := casino.SetCasino(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}

	// Without the votes there is no spin.
	if _, err := casino.Spin(ctx, ann); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Spin without votes: %v", err)
	}

	if _, err := w.users.Credit(ctx, ann, 15, 0); err != nil {
		t.Fatal(err)
	}

	if spin, err := casino.Spin(ctx, ann); err != nil || spin != (service.Spin{Prize: 0, Votes: 0, Balance: 5}) {
		t.Fatalf("losing Spin = %+v, %v", spin, err)
	}
	if _, err := casino.Spin(ctx, ann); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Spin with 5 votes: %v", err)
	}

	if _, err := w.users.Credit(ctx, ann, 5, 0); err != nil {
		t.Fatal(err)
	}
	if spin, err := casino.Spin(ctx, ann); err != nil || spin != (service.Spin{Prize: 1, Votes: 25, Balance: 25}) {
		t.Fatalf("winning Spin = %+v, %v", spin, err)
	}
	if got := w.votes(t, ann); got != 25 {
		t.Fatalf("votes = %d; want 25", got)
	}

	settings, err := casino.Settings(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Stats != (user.CasinoStats{Spins: 2, Spent: 20, Won: 25}) || len(settings.Casino.Prizes()) != 2 {
		t.Fatalf("Settings = %+v", settings)
	}
}
