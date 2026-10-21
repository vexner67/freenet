package postgres_test

import (
	"context"
	"reflect"
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

	before, err := casinos.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.CountSpin(ctx, 3, 1000); err != nil {
		t.Fatal(err)
	}
	after, err := casinos.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stats.Spins-before.Stats.Spins != 1 || after.Stats.Spent-before.Stats.Spent != 3 || after.Stats.Won-before.Stats.Won != 1000 ||
		after.Casino.Price() != 3 || !reflect.DeepEqual(after.Bets, user.DefaultCasinoBets()) {
		t.Fatalf("Settings went from %+v to %+v", before, after)
	}

	if got, err := casinos.Bets(ctx); err != nil || !reflect.DeepEqual(got, user.DefaultCasinoBets()) {
		t.Fatalf("Bets = %+v, %v; want the default", got, err)
	}

	bets, err := user.NewCasinoBets(5, 500, []user.AimMultiplier{{Multiplier: 200, Chance: 4500}, {Multiplier: 1500, Chance: 300}})
	if err != nil {
		t.Fatal(err)
	}
	if err = casinos.SetBets(ctx, bets); err != nil {
		t.Fatal(err)
	}
	if got, err := casinos.Bets(ctx); err != nil || !reflect.DeepEqual(got, bets) {
		t.Fatalf("Bets = %+v, %v; want %+v", got, err, bets)
	}

	if err = casinos.CountBet(ctx, 50, 100); err != nil {
		t.Fatal(err)
	}
	bet, err := casinos.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bet.Stats.Wagers-after.Stats.Wagers != 1 || bet.Stats.WagerSpent-after.Stats.WagerSpent != 50 ||
		bet.Stats.WagerWon-after.Stats.WagerWon != 100 || bet.Stats.Spins != after.Stats.Spins || !reflect.DeepEqual(bet.Bets, bets) {
		t.Fatalf("Settings went from %+v to %+v", after, bet)
	}

	// Without the row the bets are kept all the same, with the default
	// casino; the cleanup puts the row back as the migration made it.
	if _, err = pool.Exec(ctx, "DELETE FROM casino"); err != nil {
		t.Fatal(err)
	}
	if err = casinos.SetBets(ctx, bets); err != nil {
		t.Fatal(err)
	}
	if got, err := casinos.Settings(ctx); err != nil || !reflect.DeepEqual(got.Bets, bets) || got.Casino.Price() != user.DefaultCasino().Price() {
		t.Fatalf("Settings after SetBets without the row = %+v, %v", got, err)
	}
}
