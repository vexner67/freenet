package user

import "errors"

// The bounds of the casino: how many prizes it has — the cells of the
// field — and how big a weight may be, so that the weights add up in int64.
const (
	MaxCasinoPrizes = 12
	MaxCasinoWeight = 1_000_000_000
)

var (
	ErrCasinoPrice   = errors.New("a spin must cost at least 1 vote")
	ErrCasinoPrizes  = errors.New("the casino must have from 2 to 12 prizes")
	ErrCasinoPrize   = errors.New("a prize must be at least 0 votes")
	ErrCasinoWeight  = errors.New("the weight of a prize must be from 0 to 1000000000")
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
	if price < 1 || price > MaxAmount {
		return Casino{}, ErrCasinoPrice
	}
	if len(prizes) < 2 || len(prizes) > MaxCasinoPrizes {
		return Casino{}, ErrCasinoPrizes
	}

	var total int64

	for _, p := range prizes {
		if p.Votes < 0 || p.Votes > MaxAmount {
			return Casino{}, ErrCasinoPrize
		}
		if p.Weight < 0 || p.Weight > MaxCasinoWeight {
			return Casino{}, ErrCasinoWeight
		}

		total += int64(p.Weight)
	}

	if total == 0 {
		return Casino{}, ErrCasinoNoOdds
	}

	return Casino{price: price, prizes: append([]CasinoPrize(nil), prizes...)}, nil
}

// DefaultCasino is the casino until the administrators change it: a spin
// for 10 votes gives back 85% of them, a thousand once in 1000 spins.
func DefaultCasino() Casino {
	return Casino{price: 10, prizes: []CasinoPrize{
		{Votes: 5, Weight: 200_000},
		{Votes: 0, Weight: 89_500},
		{Votes: 1000, Weight: 1_000},
		{Votes: 0, Weight: 89_500},
		{Votes: 20, Weight: 80_000},
		{Votes: 2, Weight: 200_000},
		{Votes: 0, Weight: 89_500},
		{Votes: 300, Weight: 3_000},
		{Votes: 10, Weight: 120_000},
		{Votes: 0, Weight: 89_500},
		{Votes: 100, Weight: 10_000},
		{Votes: 50, Weight: 28_000},
	}}
}

// MaxPrize is the biggest prize of a spin.
func (c Casino) MaxPrize() int64 {
	var most int64
	for _, p := range c.prizes {
		if p.Weight > 0 {
			most = max(most, p.Votes)
		}
	}

	return most
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
// won, the same of the bets — wagers made, votes bet and won — and of the
// rounds of "Ракетка".
type CasinoStats struct {
	Spins       int64
	Spent       int64
	Won         int64
	Wagers      int64
	WagerSpent  int64
	WagerWon    int64
	Rockets     int64
	RocketSpent int64
	RocketWon   int64
}
