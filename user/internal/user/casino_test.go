package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewCasino(t *testing.T) {
	t.Parallel()

	two := []user.CasinoPrize{{Votes: 0, Weight: 1}, {Votes: 5, Weight: 1}}
	tests := []struct {
		name   string
		price  int64
		prizes []user.CasinoPrize
		want   error
	}{
		{name: "fine", price: 10, prizes: two},
		{name: "free", price: 0, prizes: two, want: user.ErrCasinoPrice},
		{name: "too dear", price: user.MaxCasinoPrice + 1, prizes: two, want: user.ErrCasinoPrice},
		{name: "one prize", price: 10, prizes: two[:1], want: user.ErrCasinoPrizes},
		{name: "too many prizes", price: 10, prizes: make([]user.CasinoPrize, user.MaxCasinoPrizes+1), want: user.ErrCasinoPrizes},
		{name: "a negative prize", price: 10, prizes: []user.CasinoPrize{{Votes: -1, Weight: 1}, {Weight: 1}}, want: user.ErrCasinoPrize},
		{name: "a huge prize", price: 10, prizes: []user.CasinoPrize{{Votes: user.MaxCasinoPrize + 1, Weight: 1}, {Weight: 1}}, want: user.ErrCasinoPrize},
		{name: "a negative weight", price: 10, prizes: []user.CasinoPrize{{Weight: -1}, {Weight: 1}}, want: user.ErrCasinoWeight},
		{name: "no odds", price: 10, prizes: []user.CasinoPrize{{Votes: 5}, {Votes: 10}}, want: user.ErrCasinoNoOdds},
		{name: "85%", price: 10, prizes: []user.CasinoPrize{{Votes: 0, Weight: 3}, {Votes: 17, Weight: 2}}},
		{name: "more than 85%", price: 10, prizes: []user.CasinoPrize{{Votes: 0, Weight: 1}, {Votes: 18, Weight: 1}}, want: user.ErrCasinoReturn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := user.NewCasino(tt.price, tt.prizes)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCasino: %v; want %v", err, tt.want)
			}
			if err == nil && (c.Price() != tt.price || len(c.Prizes()) != len(tt.prizes)) {
				t.Fatalf("NewCasino = %+v", c)
			}
		})
	}
}

func TestCasino_Pick(t *testing.T) {
	t.Parallel()

	c, err := user.NewCasino(10, []user.CasinoPrize{{Votes: 0, Weight: 2}, {Votes: 5, Weight: 0}, {Votes: 20, Weight: 1}})
	if err != nil {
		t.Fatal(err)
	}

	if c.TotalWeight() != 3 {
		t.Fatalf("TotalWeight = %d", c.TotalWeight())
	}

	// A prize of no weight is never won.
	for roll, want := range []int{0, 0, 2} {
		if got, err := c.Pick(int64(roll)); err != nil || got != want {
			t.Fatalf("Pick(%d) = %d, %v; want %d", roll, got, err, want)
		}
	}
	for _, roll := range []int64{-1, 3} {
		if _, err := c.Pick(roll); !errors.Is(err, user.ErrCasinoRollOut) {
			t.Fatalf("Pick(%d): %v", roll, err)
		}
	}
}

func TestDefaultCasino(t *testing.T) {
	t.Parallel()

	d := user.DefaultCasino()
	if _, err := user.NewCasino(d.Price(), d.Prizes()); err != nil {
		t.Fatalf("the default casino is not valid: %v", err)
	}
	if d.MaxPrize() != user.MaxWin {
		t.Fatalf("MaxPrize = %d", d.MaxPrize())
	}

	// A prize nobody wins is not the biggest.
	c, err := user.NewCasino(10, []user.CasinoPrize{{Votes: 1000, Weight: 0}, {Votes: 5, Weight: 1}})
	if err != nil || c.MaxPrize() != 5 {
		t.Fatalf("MaxPrize = %d, %v", c.MaxPrize(), err)
	}
}
