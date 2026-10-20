package model

// Casino is the casino of the site: a spin costs Price votes and wins one
// of the prizes. Players see the votes of the prizes; the administrators
// see their weights too — the odds.
type Casino struct {
	Price  int64
	Prizes []CasinoPrize
}

// CasinoPrize is a prize in votes (0 — nothing) and, for the
// administrators, its weight among the weights of all prizes.
type CasinoPrize struct {
	Votes  int64
	Weight int
}

// CasinoSettings are the casino with its odds and how it does.
type CasinoSettings struct {
	Casino Casino
	Spins  int64
	Spent  int64
	Won    int64
}

// CasinoSpin is what one spin won: the index of the prize, its votes and
// the balance after it.
type CasinoSpin struct {
	Prize   int
	Votes   int64
	Balance int64
}
