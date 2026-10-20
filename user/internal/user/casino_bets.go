package user

import "errors"

// BetOdds are the units of a bet's chance and roll: a bet wins when a roll
// in [0, BetOdds) falls in the middle of the range, chance wide, so 4750 is
// a 47.5% chance.
const BetOdds = 10_000

// The bounds of the bets: how big a bet may be and how much of the votes
// bet the casino gives back, in percent.
const (
	MaxBet       = 1_000_000
	MaxBetReturn = 100
)

// BetMultipliers are what a bet may multiply into, in hundredths: 150 is
// ×1.5. The bigger the multiplier, the smaller the chance.
var BetMultipliers = []int{150, 200, 300, 500, 1000}

var (
	ErrBetLimits     = errors.New("bets must be from 1 to 1000000 votes, the least not above the most")
	ErrBetReturn     = errors.New("the return of bets must be from 1 to 100 percent")
	ErrBetAmount     = errors.New("the bet is out of the limits of the casino")
	ErrBetMultiplier = errors.New("unknown multiplier")
)

// CasinoBets are the bets of the casino: a player bets from Least to Most
// votes on a multiplier and either gets the bet multiplied or loses it.
// The chance of a multiplier is Return percent divided by it, so on
// average the casino gives back Return percent of what is bet.
type CasinoBets struct {
	least, most int64
	ret         int
}

func NewCasinoBets(least, most int64, ret int) (CasinoBets, error) {
	if least < 1 || most > MaxBet || least > most {
		return CasinoBets{}, ErrBetLimits
	}
	if ret < 1 || ret > MaxBetReturn {
		return CasinoBets{}, ErrBetReturn
	}

	return CasinoBets{least: least, most: most, ret: ret}, nil
}

// DefaultCasinoBets are the bets until the administrators change them.
func DefaultCasinoBets() CasinoBets {
	return CasinoBets{least: 1, most: 1000, ret: 95}
}

func (b CasinoBets) Least() int64 {
	return b.least
}

func (b CasinoBets) Most() int64 {
	return b.most
}

// Return is the percent of the votes bet the casino gives back on average.
func (b CasinoBets) Return() int {
	return b.ret
}

// Bet is a bet a player makes: the votes and the multiplier, checked
// against the bets of the casino.
type Bet struct {
	amount     int64
	multiplier int
	chance     int
}

// Bet makes a bet of amount votes on multiplier; ErrBetAmount,
// ErrBetMultiplier.
func (b CasinoBets) Bet(amount int64, multiplier int) (Bet, error) {
	if amount < b.least || amount > b.most {
		return Bet{}, ErrBetAmount
	}

	chance, err := b.Chance(multiplier)
	if err != nil {
		return Bet{}, err
	}

	return Bet{amount: amount, multiplier: multiplier, chance: chance}, nil
}

// Chance is the chance of multiplier, in BetOdds; ErrBetMultiplier.
func (b CasinoBets) Chance(multiplier int) (int, error) {
	for _, m := range BetMultipliers {
		if m == multiplier {
			return b.ret * BetOdds / multiplier, nil
		}
	}

	return 0, ErrBetMultiplier
}

func (b Bet) Amount() int64 {
	return b.amount
}

func (b Bet) Multiplier() int {
	return b.multiplier
}

// Chance is the chance of the bet, in BetOdds.
func (b Bet) Chance() int {
	return b.chance
}

// Zone is where a roll wins the bet: [low, high), chance wide in the
// middle of [0, BetOdds), so that the green zone of the bar shrinks to its
// centre as the chance falls.
func (b Bet) Zone() (low, high int) {
	low = (BetOdds - b.chance) / 2

	return low, low + b.chance
}

// Wins tells whether a roll in [0, BetOdds) wins the bet: it falls in
// Zone.
func (b Bet) Wins(roll int) bool {
	low, high := b.Zone()

	return roll >= low && roll < high
}

// Prize is what the bet pays when it wins: the votes multiplied, rounded
// down.
func (b Bet) Prize() int64 {
	return b.amount * int64(b.multiplier) / 100
}
