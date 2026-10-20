package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewCasinoBets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		least, most int64
		ret         int
		want        error
	}{
		{name: "fine", least: 1, most: 1000, ret: 95},
		{name: "one amount", least: 7, most: 7, ret: 100},
		{name: "free", least: 0, most: 10, ret: 95, want: user.ErrBetLimits},
		{name: "too big", least: 1, most: user.MaxBet + 1, ret: 95, want: user.ErrBetLimits},
		{name: "upside down", least: 10, most: 5, ret: 95, want: user.ErrBetLimits},
		{name: "no return", least: 1, most: 10, ret: 0, want: user.ErrBetReturn},
		{name: "too much return", least: 1, most: 10, ret: user.MaxBetReturn + 1, want: user.ErrBetReturn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := user.NewCasinoBets(tt.least, tt.most, tt.ret)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCasinoBets: %v; want %v", err, tt.want)
			}
			if err == nil && (b.Least() != tt.least || b.Most() != tt.most || b.Return() != tt.ret) {
				t.Fatalf("NewCasinoBets = %+v", b)
			}
		})
	}
}

func TestCasinoBets_Bet(t *testing.T) {
	t.Parallel()

	bets, err := user.NewCasinoBets(10, 100, 95)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bets.Bet(9, 200); !errors.Is(err, user.ErrBetAmount) {
		t.Fatalf("Bet below the least: %v", err)
	}
	if _, err := bets.Bet(101, 200); !errors.Is(err, user.ErrBetAmount) {
		t.Fatalf("Bet above the most: %v", err)
	}
	if _, err := bets.Bet(50, 250); !errors.Is(err, user.ErrBetMultiplier) {
		t.Fatalf("Bet on ×2.5: %v", err)
	}

	tests := []struct {
		amount     int64
		multiplier int
		chance     int
		prize      int64
	}{
		{amount: 10, multiplier: 150, chance: 6333, prize: 15},
		{amount: 11, multiplier: 150, chance: 6333, prize: 16},
		{amount: 50, multiplier: 200, chance: 4750, prize: 100},
		{amount: 100, multiplier: 1000, chance: 950, prize: 1000},
	}
	for _, tt := range tests {
		bet, err := bets.Bet(tt.amount, tt.multiplier)
		if err != nil {
			t.Fatal(err)
		}
		if bet.Chance() != tt.chance || bet.Prize() != tt.prize {
			t.Fatalf("Bet(%d, %d): chance %d, prize %d; want %d, %d", tt.amount, tt.multiplier, bet.Chance(), bet.Prize(), tt.chance, tt.prize)
		}
		// The bet wins in the middle of the rolls, chance wide.
		low, high := bet.Zone()
		if high-low != tt.chance || low+high < user.BetOdds-1 || low+high > user.BetOdds {
			t.Fatalf("Bet(%d, %d).Zone() = %d, %d", tt.amount, tt.multiplier, low, high)
		}
		if bet.Wins(low-1) || !bet.Wins(low) || !bet.Wins(high-1) || bet.Wins(high) {
			t.Fatalf("Bet(%d, %d) wins in [%d, %d) only", tt.amount, tt.multiplier, low, high)
		}
	}

	// Even a casino that gives everything back never makes a bet surer
	// than certain.
	fair, err := user.NewCasinoBets(1, 10, user.MaxBetReturn)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range user.BetMultipliers {
		if c, err := fair.Chance(m); err != nil || c > user.BetOdds {
			t.Fatalf("Chance(%d) = %d, %v", m, c, err)
		}
	}
}
