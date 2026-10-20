package model

// Casino is the casino of the site: a spin costs Price votes and wins one
// of the prizes. Players see the votes of the prizes; the administrators
// see their weights too — the odds.
type Casino struct {
	Price  int64
	Prizes []CasinoPrize
	Bets   CasinoBets
}

// CasinoBets are the bets of the casino: a player bets from Least to Most
// votes on one of the Multipliers (in hundredths: 200 — ×2), which wins
// with the chance Return percent divided by it.
type CasinoBets struct {
	Least       int64
	Most        int64
	Return      int
	Multipliers []int
}

// CasinoPrize is a prize in votes (0 — nothing) and, for the
// administrators, its weight among the weights of all prizes.
type CasinoPrize struct {
	Votes  int64
	Weight int
}

// CasinoSettings are the casino with its odds and how it does: spins
// made, votes spent and won, and the same of the bets — wagers made,
// votes bet and won.
type CasinoSettings struct {
	Casino     Casino
	Spins      int64
	Spent      int64
	Won        int64
	Wagers     int64
	WagerSpent int64
	WagerWon   int64
}

// CasinoSpin is what one spin won: the index of the prize, its votes and
// the balance after it.
type CasinoSpin struct {
	Prize   int
	Votes   int64
	Balance int64
}

// CasinoWager is how one bet went: whether it won, the roll and the
// chance (both of 10000), the votes won and the balance after it.
type CasinoWager struct {
	Won     bool
	Roll    int
	Chance  int
	Votes   int64
	Balance int64
}
