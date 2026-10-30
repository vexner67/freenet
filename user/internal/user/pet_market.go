package user

import (
	"errors"
	"time"
)

// The market of pets: a person puts a pet up for votes and another buys
// it, the treasury taking PetMarketFee percent. So that the market does
// not undo the balance of the cases — no buying of the legends the cases
// give rarely — a pet goes up no sooner than PetResaleAfter after its
// owner got it and no cheaper than the least of its rarity, and a pet up
// for sale does not fight.

const (
	// PetResaleAfter is how long after its owner got it a pet may go up
	// for sale.
	PetResaleAfter = 7 * 24 * time.Hour
	// PetMarketFee is the percent of a sale the treasury takes.
	PetMarketFee = 10
	// MaxPetSales are the most pets a person keeps up for sale at once.
	MaxPetSales = 3
)

var (
	ErrPetOnSale       = errors.New("the pet is up for sale")
	ErrPetNotOnSale    = errors.New("the pet is not for sale")
	ErrPetResale       = errors.New("a pet goes up for sale a week after its owner got it")
	ErrPetSalePrice    = errors.New("a pet costs from the least of its rarity")
	ErrTooManyPetSales = errors.New("a page sells at most 3 pets at a time")
	ErrPetOwnSale      = errors.New("this is your own pet")
)

// minPetPrices are the least a pet of each rarity goes for: cheaper, the
// market would hand the rare kinds over for nothing.
var minPetPrices = map[PetRarity]int64{PetCommon: 50, PetRare: 200, PetEpic: 800, PetLegendary: 3000, PetNamed: 3000}

// MinPrice is the least a pet of the rarity goes for on the market.
func (r PetRarity) MinPrice() int64 {
	return minPetPrices[r]
}

// PetSaleFee is what the treasury takes of a sale at price.
func PetSaleFee(price int64) int64 {
	return price * PetMarketFee / 100
}

// SaleAt is when the pet may go up for sale.
func (p *Pet) SaleAt() time.Time {
	return p.OwnedSince().Add(PetResaleAfter)
}

// ListedFor is the pet up for sale at price votes at now. ErrPetResale
// before SaleAt, ErrPetSalePrice below the least of its rarity.
func (p *Pet) ListedFor(price int64, now time.Time) (*Pet, error) {
	if now.Before(p.SaleAt()) {
		return nil, ErrPetResale
	}
	if price < p.s.Kind.Rarity().MinPrice() || price > MaxAmount {
		return nil, ErrPetSalePrice
	}

	s := p.State()
	s.SalePrice = price

	return &Pet{s: s}, nil
}

// Unlisted is the pet no longer for sale.
func (p *Pet) Unlisted() *Pet {
	s := p.State()
	s.SalePrice = 0

	return &Pet{s: s}
}

// SoldAt is the pet as its buyer gets it at now: theirs since now, rested.
func (p *Pet) SoldAt(now time.Time) (*Pet, error) {
	if p.s.SalePrice == 0 {
		return nil, ErrPetNotOnSale
	}

	s := p.State()
	s.SalePrice = 0
	s.OwnedSince = now
	s.Streak, s.StreakAt = 0, time.Time{}

	return &Pet{s: s}, nil
}
