package user

import "errors"

// What the treasury takes back from the votes that change hands, and what
// the rating costs: the sinks that keep the votes of the site from piling
// up.
const (
	// TransferFeePercent is what a sender pays the treasury on top of a
	// transfer, rounded up: at least one vote.
	TransferFeePercent = 3
	// StickerTaxPercent is what the treasury takes of a sale of a sticker
	// pack of a person: the author keeps the rest, rounded down.
	StickerTaxPercent = 10
	// RatingPrice is what one percent of rating costs, in votes.
	RatingPrice = 3
	// MaxRating is the most rating a page may buy up to.
	MaxRating = 1000
	// DefaultJackpot is the win the casino shows as a jackpot until the
	// administrators set another.
	DefaultJackpot = MaxWin
)

var (
	ErrRatingRaiseInvalid = errors.New("the rating is raised by 1 to 1000 percent")
	ErrRatingLimit        = errors.New("the rating cannot go above 1000 percent")
	ErrJackpotInvalid     = errors.New("the jackpot must be from 1 to 1000 votes")
)

// TransferFee is what a transfer of votes costs its sender on top.
func TransferFee(votes int64) int64 {
	return (votes*TransferFeePercent + 99) / 100
}

// StickerTax is what the treasury takes of a sale of a sticker pack of a
// person for price.
func StickerTax(price int64) int64 {
	return price - price*(100-StickerTaxPercent)/100
}

// RatingRaise is how many percent of rating a page buys at once.
type RatingRaise int

func NewRatingRaise(percent int) (RatingRaise, error) {
	if percent < 1 || percent > MaxRating {
		return 0, ErrRatingRaiseInvalid
	}

	return RatingRaise(percent), nil
}

func (r RatingRaise) Int() int { return int(r) }

// Cost is what the raise costs, in votes.
func (r RatingRaise) Cost() int64 {
	return int64(r) * RatingPrice
}

// Jackpot is the least win the casino celebrates on the whole screen.
type Jackpot int64

func NewJackpot(votes int64) (Jackpot, error) {
	if votes < 1 || votes > MaxWin {
		return 0, ErrJackpotInvalid
	}

	return Jackpot(votes), nil
}

func (j Jackpot) Int64() int64 { return int64(j) }
