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

	if c, b, err := casino.Casino(ctx); err != nil || c.Price() != user.DefaultCasino().Price() || b != user.DefaultCasinoBets() {
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

func TestCasinoService_Bet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// The roll is the next of these.
	rolls := []int64{4749, 4750}
	casino := service.NewCasinoService(w.users, inmem.NewCasinoRepository(), w.users, inmem.TxManager{}, admins).
		WithRoll(func(n int64) (int64, error) {
			if n != user.BetOdds {
				t.Fatalf("rolled below %d", n)
			}
			roll := rolls[0]
			rolls = rolls[1:]

			return roll, nil
		})

	if _, b, err := casino.Casino(ctx); err != nil || b != user.DefaultCasinoBets() {
		t.Fatalf("Bets = %+v, %v", b, err)
	}

	// Only the administrators change the bets.
	bets, err := user.NewCasinoBets(10, 100, 95)
	if err != nil {
		t.Fatal(err)
	}
	if err := casino.SetBets(ctx, ann, bets); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetBets by ann: %v", err)
	}
	if err := casino.SetBets(ctx, admin, bets); err != nil {
		t.Fatal(err)
	}

	if _, err := casino.Bet(ctx, ann, 5, 200); !errors.Is(err, user.ErrBetAmount) {
		t.Fatalf("Bet below the least: %v", err)
	}
	if _, err := casino.Bet(ctx, ann, 50, 123); !errors.Is(err, user.ErrBetMultiplier) {
		t.Fatalf("Bet on ×1.23: %v", err)
	}
	if _, err := casino.Bet(ctx, ann, 50, 200); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Bet without votes: %v", err)
	}

	if _, err := w.users.Credit(ctx, ann, 100, 0); err != nil {
		t.Fatal(err)
	}

	want := service.Wager{Won: true, Roll: 4749, Chance: 4750, Votes: 100, Balance: 150}
	if wager, err := casino.Bet(ctx, ann, 50, 200); err != nil || wager != want {
		t.Fatalf("winning Bet = %+v, %v; want %+v", wager, err, want)
	}

	want = service.Wager{Roll: 4750, Chance: 4750, Balance: 50}
	if wager, err := casino.Bet(ctx, ann, 100, 200); err != nil || wager != want {
		t.Fatalf("losing Bet = %+v, %v; want %+v", wager, err, want)
	}
	if got := w.votes(t, ann); got != 50 {
		t.Fatalf("votes = %d; want 50", got)
	}

	settings, err := casino.Settings(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Bets != bets || settings.Stats != (user.CasinoStats{Wagers: 2, WagerSpent: 150, WagerWon: 100}) {
		t.Fatalf("Settings = %+v", settings)
	}
}
