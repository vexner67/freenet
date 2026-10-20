package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCasinoRepository(t *testing.T) {
	_, pool := newRepo(t)
	casinos := postgres.NewCasinoRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it as the default casino; the test leaves it so.
	t.Cleanup(func() {
		if err := casinos.SetCasino(context.Background(), user.DefaultCasino()); err != nil {
			t.Error(err)
		}
		if err := casinos.SetBets(context.Background(), user.DefaultCasinoBets()); err != nil {
			t.Error(err)
		}
	})

	got, err := casinos.Casino(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := user.DefaultCasino(); got.Price() != want.Price() || len(got.Prizes()) != len(want.Prizes()) || got.TotalWeight() != want.TotalWeight() {
		t.Fatalf("Casino = %+v; want the default %+v", got, want)
	}

	tuned, err := user.NewCasino(3, []user.CasinoPrize{{Votes: 0, Weight: 9}, {Votes: 1000, Weight: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.SetCasino(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err = casinos.Casino(ctx); err != nil || got.Price() != 3 || got.Prizes()[1] != (user.CasinoPrize{Votes: 1000, Weight: 1}) {
		t.Fatalf("Casino = %+v, %v; want the tuned one", got, err)
	}

	before, err := casinos.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.CountSpin(ctx, 3, 1000); err != nil {
		t.Fatal(err)
	}
	after, err := casinos.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Spins-before.Spins != 1 || after.Spent-before.Spent != 3 || after.Won-before.Won != 1000 {
		t.Fatalf("Stats went from %+v to %+v", before, after)
	}

	if got, err := casinos.Bets(ctx); err != nil || got != user.DefaultCasinoBets() {
		t.Fatalf("Bets = %+v, %v; want the default", got, err)
	}

	bets, err := user.NewCasinoBets(5, 500, 90)
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.SetBets(ctx, bets); err != nil {
		t.Fatal(err)
	}
	if got, err := casinos.Bets(ctx); err != nil || got != bets {
		t.Fatalf("Bets = %+v, %v; want %+v", got, err, bets)
	}

	if err = casinos.CountBet(ctx, 50, 100); err != nil {
		t.Fatal(err)
	}
	bet, err := casinos.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bet.Bets-after.Bets != 1 || bet.BetSpent-after.BetSpent != 50 || bet.BetWon-after.BetWon != 100 || bet.Spins != after.Spins {
		t.Fatalf("Stats went from %+v to %+v", after, bet)
	}
}
