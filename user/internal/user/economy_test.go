package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestTransferFee(t *testing.T) {
	t.Parallel()

	for votes, want := range map[int64]int64{1: 1, 33: 1, 34: 2, 100: 3, 101: 4, 1000: 30, user.MaxVotes: 3_000_000} {
		if got := user.TransferFee(votes); got != want {
			t.Errorf("TransferFee(%d) = %d, want %d", votes, got, want)
		}
	}
}

func TestStickerTax(t *testing.T) {
	t.Parallel()

	for price, want := range map[int64]int64{1: 1, 9: 1, 10: 1, 11: 2, 50: 5, 99: 10, 1000: 100} {
		if got := user.StickerTax(price); got != want {
			t.Errorf("StickerTax(%d) = %d, want %d", price, got, want)
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

	for _, bad := range []int64{0, user.MaxWin + 1} {
		if _, err := user.NewJackpot(bad); !errors.Is(err, user.ErrJackpotInvalid) {
			t.Errorf("NewJackpot(%d) = %v", bad, err)
		}
	}

	if j, err := user.NewJackpot(500); err != nil || j.Int64() != 500 {
		t.Fatalf("NewJackpot(500) = %v, %v", j, err)
	}
}
