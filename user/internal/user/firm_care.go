package user

import (
	"math"
	"slices"

	"github.com/google/uuid"
)

// Improvement is what an enterprise buys of its cash to do better, rank
// by rank: machines make more, a storage holds more, and sales get better
// prices of the state and the city.
type Improvement string

const (
	ImproveMachines Improvement = "machines"
	ImproveStorage  Improvement = "storage"
	ImproveSales    Improvement = "sales"
)

// Improvements are all improvements.
var Improvements = []Improvement{ImproveMachines, ImproveStorage, ImproveSales}

const (
	// MaxRank is the top rank of an improvement.
	MaxRank = 3
	// wearPerHour is what an hour of a full staff wears the machines.
	wearPerHour = 0.4
	// wornFrom is the wear the machines bear with no loss; past it the
	// output falls to wornOutput at 100.
	wornFrom   = 30.0
	wornOutput = 0.4
	// repairPerMille is the votes a point of wear costs to repair, per
	// mille of what founding and the levels cost.
	repairPerMille = 1
)

func NewImprovement(s string) (Improvement, error) {
	for _, imp := range Improvements {
		if string(imp) == s {
			return imp, nil
		}
	}

	return "", ErrImprovement
}

// ImprovementCost is the votes the rank after rank of an improvement costs
// an enterprise of plan: half its price times the rank.
func ImprovementCost(plan FirmPlan, rank int) int64 {
	return plan.Price * int64(rank+1) / 2
}

// RepairCost is the hundredths repairing the machines of the enterprise
// costs: a thousandth of what founding and its levels cost a point.
func RepairCost(plan FirmPlan, f *Firm) int64 {
	return int64(math.Ceil(f.wear * float64(plan.Invested(f.level)) * repairPerMille * 100 / 1000))
}

// condition is how much the machines give of what new ones would.
func (f *Firm) condition() float64 {
	if f.wear <= wornFrom {
		return 1
	}

	return 1 - (f.wear-wornFrom)/(100-wornFrom)*(1-wornOutput)
}

// machines is what the rank of the machines adds to the output.
func (f *Firm) machines() float64 {
	return 1 + 0.15*float64(f.ranks[ImproveMachines])
}

// sales is what the rank of the sales adds to the prices of the state and
// the city.
func (f *Firm) sales() float64 {
	return 1 + 0.05*float64(f.ranks[ImproveSales])
}

// storage is what the rank of the storage adds to what it holds.
func (f *Firm) storage() float64 {
	return 1 + 0.5*float64(f.ranks[ImproveStorage])
}

// wearOut wears the machines by an hour of workers of seats.
func (f *Firm) wearOut(workers float64, seats int) {
	if seats > 0 {
		f.wear = min(f.wear+wearPerHour*workers/float64(seats), 100)
	}
}

// Repair repairs the machines of the enterprise of its cash; ErrFirmWorn,
// ErrFirmCash.
func (c *City) Repair(rules FirmRules, userID, firmID uuid.UUID) (int64, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return 0, err
	}

	plan, err := rules.Plan(f.kind)
	if err != nil {
		return 0, err
	}

	cost := RepairCost(plan, f)
	if f.wear < 1 {
		return 0, ErrFirmWorn
	}
	if f.cash < cost {
		return 0, ErrFirmCash
	}

	f.cash -= cost
	f.wear = 0

	return cost, nil
}

// Improve raises the rank of an improvement of the enterprise, paid of its
// cash; half of it counts in what the enterprise is worth closed.
// ErrImprovement, ErrFirmCash.
func (c *City) Improve(rules FirmRules, userID, firmID uuid.UUID, imp Improvement) (int64, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return 0, err
	}

	plan, err := rules.Plan(f.kind)
	if err != nil {
		return 0, err
	}

	rank := f.ranks[imp]
	if !slices.Contains(Improvements, imp) || rank >= MaxRank {
		return 0, ErrImprovement
	}

	cost := ImprovementCost(plan, rank) * 100
	if f.cash < cost {
		return 0, ErrFirmCash
	}

	f.cash -= cost
	f.ranks[imp] = rank + 1
	f.improved += cost / 100

	return cost, nil
}
