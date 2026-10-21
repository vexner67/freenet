package model

// Casino is the casino of the site: a spin costs Price votes and wins one
// of the prizes. Players see the votes of the prizes; the administrators
// see their weights too — the odds.
type Casino struct {
	Price  int64
	Prizes []CasinoPrize
	Bets   CasinoBets
	Rocket CasinoRocket
}

// CasinoRocket is "Ракетка": bets from Least to Most votes, a rocket that
// explodes at Top (in hundredths, 100000 — ×1000) at the latest and, for
// the administrators, gives back Return of the bets on average, in
// hundredths of a percent (9500 — 95%).
type CasinoRocket struct {
	Least  int64
	Most   int64
	Return int
	Top    int
}

// CasinoBets are the bets of the casino: a player bets from Least to Most
// votes on one of the Multipliers, each winning with its chance.
type CasinoBets struct {
	Least       int64
	Most        int64
	Multipliers []AimMultiplier
}

// AimMultiplier is a multiplier of the bets in hundredths (200 — ×2) and
// the chance it wins with, of 10000.
type AimMultiplier struct {
	Multiplier int
	Chance     int
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
	// Rockets are the rounds of "Ракетка" launched, RocketSpent and
	// RocketWon — the votes they took and paid.
	Rockets     int64
	RocketSpent int64
	RocketWon   int64
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

// RocketState is where a round of "Ракетка" is.
type RocketState string

const (
	RocketFlying  RocketState = "flying"
	RocketCashed  RocketState = "cashed"
	RocketCrashed RocketState = "crashed"
)

// RocketRound is a round of "Ракетка" as its player sees it: the bet, the
// multiplier it cashes at by itself (0 — by hand), the multiplier it was
// cashed at and what it paid, the crash point once it is over (0 while it
// flies), how long it has flown and the balance after the launch or the
// cash (0 when it was only read).
type RocketRound struct {
	ID        string
	Amount    int64
	Auto      int
	State     RocketState
	Cashed    int
	Payout    int64
	Crash     int
	ElapsedMS int64
	Balance   int64
}
