package user

import "errors"

// What the rating costs and what the casino celebrates.
const (
	// RatingPrice is what one percent of rating costs, in votes.
	RatingPrice = 3
	// MaxRating is the most rating a page may buy up to.
	MaxRating = 1000
	// DefaultJackpot is the win the casino shows as a jackpot until the
	// administrators set another.
	DefaultJackpot = 1000
)

var (
	ErrRatingRaiseInvalid = errors.New("the rating is raised by 1 to 1000 percent")
	ErrRatingLimit        = errors.New("the rating cannot go above 1000 percent")
	ErrJackpotInvalid     = errors.New("the jackpot must be at least 1 vote")
	ErrEconomyBonus       = errors.New("the daily bonus must be at least 1 vote")
	ErrEconomyPercent     = errors.New("a fee must be from 0 to 100 percent")
)

// Economy is what the administrators tune of the votes that change hands:
// the daily bonus the treasury pays, and what it takes back — a fee in
// percent a sender pays on top of a transfer (rounded up; nothing with
// Prime) and a tax in percent of a sale of a sticker pack of a person (the
// author keeps the rest, rounded down), lower for an author with Prime.
type Economy struct {
	Bonus           int64
	TransferFee     int
	StickerTax      int
	PrimeStickerTax int
}

// DefaultEconomy is the economy until the administrators change it.
var DefaultEconomy = Economy{Bonus: 10, TransferFee: 3, StickerTax: 10, PrimeStickerTax: 5}

// NewEconomy checks an economy; ErrEconomyBonus, ErrEconomyPercent.
func NewEconomy(bonus int64, transferFee, stickerTax, primeStickerTax int) (Economy, error) {
	if bonus < 1 || bonus > MaxAmount {
		return Economy{}, ErrEconomyBonus
	}
	for _, p := range []int{transferFee, stickerTax, primeStickerTax} {
		if p < 0 || p > 100 {
			return Economy{}, ErrEconomyPercent
		}
	}

	return Economy{Bonus: bonus, TransferFee: transferFee, StickerTax: stickerTax, PrimeStickerTax: primeStickerTax}, nil
}

// TransferFeeOf is what a transfer of votes costs its sender on top.
func (e Economy) TransferFeeOf(votes int64) int64 {
	return (votes*int64(e.TransferFee) + 99) / 100
}

// StickerTaxOf is what the treasury takes of a sale of a sticker pack for
// price, from an author with Prime or not.
func (e Economy) StickerTaxOf(price int64, prime bool) int64 {
	percent := int64(e.StickerTax)
	if prime {
		percent = int64(e.PrimeStickerTax)
	}

	return price - price*(100-percent)/100
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
	if votes < 1 || votes > MaxAmount {
		return 0, ErrJackpotInvalid
	}

	return Jackpot(votes), nil
}

func (j Jackpot) Int64() int64 { return int64(j) }
