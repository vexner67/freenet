package user

import (
	"errors"
	"slices"
)

// BetOdds are the units of a bet's chance and roll: a bet wins when a roll
// in [0, BetOdds) falls in the middle of the range, chance wide, so 4750 is
// a 47.5% chance.
const BetOdds = 10_000

// The bounds of the bets: how big a bet may be, how many multipliers it
// chooses from and how big they are, in hundredths: 101 is ×1.01.
const (
	MaxBet            = MaxAmount
	MaxAimMultipliers = 10
	MinAimMultiplier  = 101
	MaxAimMultiplier  = MaxMultiplier
)

// defaultReturn is what the default games give back on average, in
// percent.
const defaultReturn = 97

// defaultAim are the multipliers until the administrators change them,
// each winning with the chance 97% divided by it: the casino gives back
// 97% of what is bet on average, as real casinos do.
var defaultAim = []int{200, 300, 400, 500, 700, 1000}

var (
	ErrBetLimits          = errors.New("bets must be at least 1 vote, the least not above the most")
	ErrAimMultipliers     = errors.New("bets have from 1 to 10 multipliers")
	ErrAimMultiplier      = errors.New("a multiplier must be from 1.01 to 10000")
	ErrAimChance          = errors.New("the chance of a multiplier must be from 0.01 to 99.99 percent")
	ErrAimMultiplierTwice = errors.New("the multipliers repeat")
	ErrBetAmount          = errors.New("the bet is out of the limits of the casino")
	ErrBetMultiplier      = errors.New("unknown multiplier")
)

// AimMultiplier is what a bet may multiply into, in hundredths — 150 is
// ×1.5 — and the chance it does, in BetOdds: 4750 is 47.5%. On average a
// bet on it gives back Multiplier × Chance / 100 / BetOdds of itself.
type AimMultiplier struct {
	Multiplier int
	Chance     int
}

// CasinoBets are the bets of the casino: a player bets from Least to Most
// votes on one of the multipliers and either gets the bet multiplied or
// loses it; the administrators set each multiplier and its chance.
type CasinoBets struct {
	least, most int64
	aim         []AimMultiplier
}

// NewCasinoBets checks the bets; the multipliers come smallest first.
// ErrBetLimits, ErrAimMultipliers, ErrAimMultiplier, ErrAimChance,
// ErrAimMultiplierTwice.
func NewCasinoBets(least, most int64, aim []AimMultiplier) (CasinoBets, error) {
	if least < 1 || most > MaxBet || least > most {
		return CasinoBets{}, ErrBetLimits
	}
	if len(aim) < 1 || len(aim) > MaxAimMultipliers {
		return CasinoBets{}, ErrAimMultipliers
	}

	sorted := slices.Clone(aim)
	slices.SortFunc(sorted, func(a, b AimMultiplier) int { return a.Multiplier - b.Multiplier })
	for i, m := range sorted {
		switch {
		case m.Multiplier < MinAimMultiplier || m.Multiplier > MaxAimMultiplier:
			return CasinoBets{}, ErrAimMultiplier
		case m.Chance < 1 || m.Chance >= BetOdds:
			return CasinoBets{}, ErrAimChance
		case i > 0 && sorted[i-1].Multiplier == m.Multiplier:
			return CasinoBets{}, ErrAimMultiplierTwice
		}
	}

	return CasinoBets{least: least, most: most, aim: sorted}, nil
}

// DefaultCasinoBets are the bets until the administrators change them.
func DefaultCasinoBets() CasinoBets {
	return CasinoBets{least: 1, most: 100, aim: DefaultAim()}
}

// DefaultAim are the multipliers of DefaultCasinoBets.
func DefaultAim() []AimMultiplier {
	aim := make([]AimMultiplier, len(defaultAim))
	for i, m := range defaultAim {
		aim[i] = AimMultiplier{Multiplier: m, Chance: defaultReturn * BetOdds / m}
	}

	return aim
}

func (b CasinoBets) Least() int64 {
	return b.least
}

func (b CasinoBets) Most() int64 {
	return b.most
}

// Aim are the multipliers a bet may take with their chances, smallest
// first.
func (b CasinoBets) Aim() []AimMultiplier {
	return slices.Clone(b.aim)
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
	for _, m := range b.aim {
		if m.Multiplier == multiplier {
			return m.Chance, nil
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
