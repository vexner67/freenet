package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestEconomy_TransferFeeOf(t *testing.T) {
	t.Parallel()

	e := user.DefaultEconomy
	for votes, want := range map[int64]int64{1: 1, 33: 1, 34: 2, 100: 3, 101: 4, 1000: 30, user.MaxVotes: user.MaxVotes / 100 * 3} {
		if got := e.TransferFeeOf(votes); got != want {
			t.Errorf("TransferFeeOf(%d) = %d, want %d", votes, got, want)
		}
	}

	if free := (user.Economy{Bonus: 1}).TransferFeeOf(100); free != 0 {
		t.Errorf("a fee of 0%% = %d", free)
	}
}

func TestEconomy_StickerTaxOf(t *testing.T) {
	t.Parallel()

	e := user.DefaultEconomy
	for price, want := range map[int64]int64{1: 1, 9: 1, 10: 1, 11: 2, 50: 5, 99: 10, 1000: 100} {
		if got := e.StickerTaxOf(price, false); got != want {
			t.Errorf("StickerTaxOf(%d) = %d, want %d", price, got, want)
		}
	}
	if got := e.StickerTaxOf(100, true); got != 5 {
		t.Errorf("StickerTaxOf with Prime = %d", got)
	}
}

func TestNewEconomy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		bonus              int64
		fee, tax, primeTax int
		want               error
	}{
		{name: "fine", bonus: 10_000, fee: 3, tax: 10, primeTax: 5},
		{name: "free", bonus: 1, fee: 0, tax: 0, primeTax: 0},
		{name: "all", bonus: user.MaxAmount, fee: 100, tax: 100, primeTax: 100},
		{name: "no bonus", bonus: 0, want: user.ErrEconomyBonus},
		{name: "a huge bonus", bonus: user.MaxAmount + 1, want: user.ErrEconomyBonus},
		{name: "a negative fee", bonus: 1, fee: -1, want: user.ErrEconomyPercent},
		{name: "a tax above 100%", bonus: 1, tax: 101, want: user.ErrEconomyPercent},
		{name: "a Prime tax above 100%", bonus: 1, primeTax: 101, want: user.ErrEconomyPercent},
	}

	for _, tt := range tests {
		e, err := user.NewEconomy(tt.bonus, tt.fee, tt.tax, tt.primeTax)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: NewEconomy = %v, want %v", tt.name, err, tt.want)
		}
		if err == nil && (e.Bonus != tt.bonus || e.TransferFee != tt.fee || e.StickerTax != tt.tax || e.PrimeStickerTax != tt.primeTax) {
			t.Errorf("%s: NewEconomy = %+v", tt.name, e)
		}
	}
}

func TestNewRatingRaise(t *testing.T) {
	t.Parallel()

	for _, bad := range []int{-1, 0, user.MaxRating + 1} {
		if _, err := user.NewRatingRaise(bad); !errors.Is(err, user.ErrRatingRaiseInvalid) {
			t.Errorf("NewRatingRaise(%d) = %v", bad, err)
		}
	}

	r, err := user.NewRatingRaise(5)
	if err != nil || r.Int() != 5 || r.Cost() != 5*user.RatingPrice {
		t.Fatalf("NewRatingRaise(5) = %v, %v", r, err)
	}
}

func TestNewJackpot(t *testing.T) {
	t.Parallel()

	for _, bad := range []int64{0, user.MaxAmount + 1} {
		if _, err := user.NewJackpot(bad); !errors.Is(err, user.ErrJackpotInvalid) {
			t.Errorf("NewJackpot(%d) = %v", bad, err)
		}
	}

	if j, err := user.NewJackpot(500); err != nil || j.Int64() != 500 {
		t.Fatalf("NewJackpot(500) = %v, %v", j, err)
	}
}

func TestBonusOf(t *testing.T) {
	t.Parallel()

	e := user.Economy{Bonus: 10}
	for day, want := range map[int]int64{0: 10, 1: 10, 2: 20, 7: 70, 8: 70, 365: 70} {
		if got := e.BonusOf(day); got != want {
			t.Errorf("BonusOf(%d) = %d, want %d", day, got, want)
		}
	}
}
