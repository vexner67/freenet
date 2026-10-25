package user

import "errors"

// BusinessPerk is an improvement of a business the owner buys in tiers:
// each tier makes the business better at one thing and shows on its
// premises.
type BusinessPerk string

const (
	// PerkDecor is the interior: more customers come.
	PerkDecor BusinessPerk = "decor"
	// PerkBrand is the signboard and the name: customers pay a dearer
	// price as fair.
	PerkBrand BusinessPerk = "brand"
	// PerkEquipment is the tools of the workers: each serves more.
	PerkEquipment BusinessPerk = "equipment"
	// PerkShelves is the racks of the storage: it holds more goods.
	PerkShelves BusinessPerk = "shelves"
	// PerkFinish is the finishing of the premises: they wear slower.
	PerkFinish BusinessPerk = "finish"
	// PerkTraining is the training of the staff: the wages are lower.
	PerkTraining BusinessPerk = "training"
)

// BusinessPerks are all improvements in order.
var BusinessPerks = []BusinessPerk{PerkDecor, PerkBrand, PerkEquipment, PerkShelves, PerkFinish, PerkTraining}

// BusinessImprove buys the next tier of an improvement.
const BusinessImprove BusinessAction = "improve"

// MaxPerkTier is the top tier of an improvement.
const MaxPerkTier = 3

// perkLevels are the levels of the business each tier opens at; a plan
// with fewer levels opens the tier at its top level.
var perkLevels = [MaxPerkTier + 1]int{0, 1, 3, 6}

var (
	ErrUnknownPerk = errors.New("unknown business improvement")
	ErrPerkTop     = errors.New("the improvement is at its top tier")
	ErrPerkLocked  = errors.New("the business level is too low for this improvement")
)

func NewBusinessPerk(s string) (BusinessPerk, error) {
	for _, p := range BusinessPerks {
		if string(p) == s {
			return p, nil
		}
	}

	return "", ErrUnknownPerk
}

func (p BusinessPerk) String() string {
	return string(p)
}

// PerkLevel is the level of the business that opens tier of an
// improvement.
func (p BusinessPlan) PerkLevel(tier int) int {
	return min(perkLevels[max(1, min(tier, MaxPerkTier))], p.Levels)
}

// PerkPrice is what tier of an improvement costs: two fifths of the price
// of the business times the tier.
func (p BusinessPlan) PerkPrice(tier int) int64 {
	return p.Price * 2 / 5 * int64(tier)
}

// Perk is the tier of an improvement of b, 0 — not bought.
func (b Business) Perk(perk BusinessPerk) int {
	return b.perks[perk]
}

// Perks are the bought tiers of the improvements of b.
func (b Business) Perks() map[BusinessPerk]int {
	perks := make(map[BusinessPerk]int, len(b.perks))
	for p, tier := range b.perks {
		perks[p] = tier
	}

	return perks
}

// perkRate is 1 plus percent per tier of perk of b.
func (b Business) perkRate(perk BusinessPerk, percent float64) float64 {
	return 1 + percent*float64(b.perks[perk])/100
}

// Storage is how many units of goods the storage of b holds: the level
// and the racks.
func (b Business) Storage(p BusinessPlan) int64 {
	return p.StorageAt(b.level) * int64(2+b.perks[PerkShelves]) / 2
}

// Improved is b with the next tier of perk and what it cost; the tier
// opens at a level of the business.
func (b Business) Improved(p BusinessPlan, perk BusinessPerk) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}

	tier := b.perks[perk] + 1
	switch {
	case tier > MaxPerkTier:
		return Business{}, 0, ErrPerkTop
	case b.level < p.PerkLevel(tier):
		return Business{}, 0, ErrPerkLocked
	}

	cost := p.PerkPrice(tier)
	b.perks = b.Perks()
	b.perks[perk] = tier
	b.invested += cost

	return b, cost, nil
}
