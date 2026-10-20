package user

import "errors"

// The bounds of the casino: what a spin may cost, how many prizes it has
// and how big a prize and a weight may be.
const (
	MaxCasinoPrice  = 10_000
	MaxCasinoPrizes = 12
	MaxCasinoPrize  = 1_000_000
	MaxCasinoWeight = 1_000_000
)

var (
	ErrCasinoPrice   = errors.New("a spin must cost from 1 to 10000 votes")
	ErrCasinoPrizes  = errors.New("the casino must have from 2 to 12 prizes")
	ErrCasinoPrize   = errors.New("a prize must be from 0 to 1000000 votes")
	ErrCasinoWeight  = errors.New("the weight of a prize must be from 0 to 1000000")
	ErrCasinoNoOdds  = errors.New("at least one prize must have a weight")
	ErrCasinoRollOut = errors.New("the roll is out of the weights")
)

// CasinoPrize is what a spin may win, in votes (0 — nothing), and how
// likely it is: its weight among the weights of all prizes.
type CasinoPrize struct {
	Votes  int64
	Weight int
}

// Casino is the casino of the site: a spin costs Price votes and wins one
// of the prizes, each as likely as its weight among all weights. The
// administrators change the prizes and their weights — the odds.
type Casino struct {
	price  int64
	prizes []CasinoPrize
}

func NewCasino(price int64, prizes []CasinoPrize) (Casino, error) {
	if price < 1 || price > MaxCasinoPrice {
		return Casino{}, ErrCasinoPrice
	}
	if len(prizes) < 2 || len(prizes) > MaxCasinoPrizes {
		return Casino{}, ErrCasinoPrizes
	}

	total := 0

	for _, p := range prizes {
		if p.Votes < 0 || p.Votes > MaxCasinoPrize {
			return Casino{}, ErrCasinoPrize
		}
		if p.Weight < 0 || p.Weight > MaxCasinoWeight {
			return Casino{}, ErrCasinoWeight
		}

		total += p.Weight
	}

	if total == 0 {
		return Casino{}, ErrCasinoNoOdds
	}

	return Casino{price: price, prizes: append([]CasinoPrize(nil), prizes...)}, nil
}

// DefaultCasino is the casino until the administrators change it: a spin
// for 10 votes gives back about 70% of them.
func DefaultCasino() Casino {
	return Casino{price: 10, prizes: []CasinoPrize{
		{Votes: 0, Weight: 50},
		{Votes: 5, Weight: 25},
		{Votes: 10, Weight: 12},
		{Votes: 20, Weight: 8},
		{Votes: 50, Weight: 4},
		{Votes: 100, Weight: 1},
	}}
}

func (c Casino) Price() int64 {
	return c.price
}

func (c Casino) Prizes() []CasinoPrize {
	return append([]CasinoPrize(nil), c.prizes...)
}

// TotalWeight is the sum of the weights: a roll is taken below it.
func (c Casino) TotalWeight() int64 {
	var total int64
	for _, p := range c.prizes {
		total += int64(p.Weight)
	}

	return total
}

// Pick is the index of the prize a roll in [0, TotalWeight) wins.
func (c Casino) Pick(roll int64) (int, error) {
	if roll < 0 {
		return 0, ErrCasinoRollOut
	}

	for i, p := range c.prizes {
		if roll < int64(p.Weight) {
			return i, nil
		}

		roll -= int64(p.Weight)
	}

	return 0, ErrCasinoRollOut
}

// CasinoStats is how the casino does: spins made, votes spent on them and
// won, and the same of the bets: wagers made, votes bet and won.
type CasinoStats struct {
	Spins      int64
	Spent      int64
	Won        int64
	Wagers     int64
	WagerSpent int64
	WagerWon   int64
}
