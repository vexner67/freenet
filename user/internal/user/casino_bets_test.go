package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewCasinoBets(t *testing.T) {
	t.Parallel()

	aim := func(pairs ...int) []user.AimMultiplier {
		out := make([]user.AimMultiplier, 0, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, user.AimMultiplier{Multiplier: pairs[i], Chance: pairs[i+1]})
		}

		return out
	}
	eleven := make([]user.AimMultiplier, 0, user.MaxAimMultipliers+1)
	for m := range user.MaxAimMultipliers + 1 {
		eleven = append(eleven, user.AimMultiplier{Multiplier: 200 + m, Chance: 100})
	}

	tests := []struct {
		name        string
		least, most int64
		aim         []user.AimMultiplier
		want        error
	}{
		{name: "fine", least: 1, most: 100, aim: aim(200, 4250, 150, 5666)},
		{name: "one amount", least: 7, most: 7, aim: aim(101, 8415)},
		{name: "returns too much", least: 1, most: 10, aim: aim(200, 4251), want: user.ErrAimReturn},
		{name: "wins too much", least: 1, most: 101, aim: aim(200, 4250, 1000, 850), want: user.ErrAimWin},
		{name: "free", least: 0, most: 10, aim: aim(200, 4750), want: user.ErrBetLimits},
		{name: "too big", least: 1, most: user.MaxBet + 1, aim: aim(200, 4750), want: user.ErrBetLimits},
		{name: "upside down", least: 10, most: 5, aim: aim(200, 4750), want: user.ErrBetLimits},
		{name: "no multipliers", least: 1, most: 10, want: user.ErrAimMultipliers},
		{name: "too many multipliers", least: 1, most: 10, aim: eleven, want: user.ErrAimMultipliers},
		{name: "×1", least: 1, most: 10, aim: aim(100, 5000), want: user.ErrAimMultiplier},
		{name: "too big a multiplier", least: 1, most: 10, aim: aim(user.MaxAimMultiplier+1, 1), want: user.ErrAimMultiplier},
		{name: "no chance", least: 1, most: 10, aim: aim(200, 0), want: user.ErrAimChance},
		{name: "certain", least: 1, most: 10, aim: aim(200, user.BetOdds), want: user.ErrAimChance},
		{name: "twice", least: 1, most: 10, aim: aim(200, 4000, 200, 4750), want: user.ErrAimMultiplierTwice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := user.NewCasinoBets(tt.least, tt.most, tt.aim)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCasinoBets: %v; want %v", err, tt.want)
			}
			if err == nil && (b.Least() != tt.least || b.Most() != tt.most || len(b.Aim()) != len(tt.aim)) {
				t.Fatalf("NewCasinoBets = %+v", b)
			}
			// The multipliers come smallest first.
			for i := 1; err == nil && i < len(b.Aim()); i++ {
				if b.Aim()[i-1].Multiplier >= b.Aim()[i].Multiplier {
					t.Fatalf("Aim() = %+v", b.Aim())
				}
			}
		})
	}
}

func TestCasinoBets_Bet(t *testing.T) {
	t.Parallel()

	bets, err := user.NewCasinoBets(10, 100, user.DefaultAim())
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
		{amount: 10, multiplier: 200, chance: 4250, prize: 20},
		{amount: 11, multiplier: 300, chance: 2833, prize: 33},
		{amount: 50, multiplier: 200, chance: 4250, prize: 100},
		{amount: 100, multiplier: 1000, chance: 850, prize: 1000},
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

	// The administrators set the chance of each multiplier as they like.
	custom, err := user.NewCasinoBets(1, 10, []user.AimMultiplier{{Multiplier: 250, Chance: 3000}})
	if err != nil {
		t.Fatal(err)
	}
	if bet, err := custom.Bet(10, 250); err != nil || bet.Chance() != 3000 || bet.Prize() != 25 {
		t.Fatalf("custom Bet = %+v, %v", bet, err)
	}
	if _, err := custom.Bet(10, 200); !errors.Is(err, user.ErrBetMultiplier) {
		t.Fatalf("Bet on a multiplier gone: %v", err)
	}
}
