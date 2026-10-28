package world

import "errors"

var ErrRules = errors.New("invalid rules of the world")

// Rules are the numbers of «Мир» the administrators may change.
type Rules struct {
	// FoundCost is the votes a state costs; they go to the treasury of
	// the site.
	FoundCost int64
	// FoundTreasury is the votes a new state has in its treasury.
	FoundTreasury int64
	// FoundLands is how many free neighbours of its capital a new state
	// gets.
	FoundLands int
	// CapitalDistance is how many provinces at least a new capital lies
	// from another.
	CapitalDistance int
	// ProtectionDays: so long nobody may make war on a new state.
	ProtectionDays int
	// ColonizeCost is the votes of the treasury a colony costs, more by a
	// tenth for every province the state has.
	ColonizeCost int64
	// ColonizeHours is how long a colony takes.
	ColonizeHours int
	// Income is the hundredths of a vote ten thousand people bring a
	// treasury in an hour.
	Income int64
}

// DefaultRules are the rules until the administrators change them.
var DefaultRules = Rules{
	FoundCost:       5000,
	FoundTreasury:   1000,
	FoundLands:      2,
	CapitalDistance: 3,
	ProtectionDays:  7,
	ColonizeCost:    200,
	ColonizeHours:   12,
	Income:          5,
}

// Validate keeps the numbers where the world still makes sense.
func (r Rules) Validate() error {
	switch {
	case r.FoundCost < 0 || r.FoundCost > 10_000_000:
		return ErrRules
	case r.FoundTreasury < 0 || r.FoundTreasury > 10_000_000:
		return ErrRules
	case r.FoundLands < 0 || r.FoundLands > 10:
		return ErrRules
	case r.CapitalDistance < 1 || r.CapitalDistance > 10:
		return ErrRules
	case r.ProtectionDays < 0 || r.ProtectionDays > 60:
		return ErrRules
	case r.ColonizeCost < 0 || r.ColonizeCost > 1_000_000:
		return ErrRules
	case r.ColonizeHours < 1 || r.ColonizeHours > 24*14:
		return ErrRules
	case r.Income < 0 || r.Income > 1000:
		return ErrRules
	}

	return nil
}

// colonizeCost is the hundredths a colony of a state with lands provinces
// costs: a tenth dearer for each.
func (r Rules) colonizeCost(lands int) int64 {
	return r.ColonizeCost * int64(10+lands) * 10
}

// colonists is how many colonies a state with lands provinces may found
// at once.
func colonists(lands int) int {
	return 1 + lands/10
}
