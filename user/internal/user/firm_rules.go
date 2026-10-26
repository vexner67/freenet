package user

import (
	"errors"
	"math"
)

// The bounds of the settings of the enterprises: not rules of the game,
// only what the arithmetic and the fields of the API hold.
const (
	MaxFirmLevels  = 100
	MaxFirmStaff   = 1_000
	MaxFirmPrice   = 100_000_000
	MaxGoodNorm    = 1_000_000_000
	MaxFirmPercent = 1_000
	MaxFirmFee     = 50
	MaxFirmStorage = 30 * 24
)

var (
	ErrFirmPlans    = errors.New("the settings must set every enterprise and every good once, in order")
	ErrFirmPlan     = errors.New("an enterprise must cost from 1 vote, an upgrade from 0, have from 1 level and 1 seat, an upkeep from 0")
	ErrGoodPlan     = errors.New("a good must cost from 1 hundredth and the state must buy from 1 unit a day")
	ErrFirmPercents = errors.New("the wage must be from 1 hundredth, the fee from 0 to 50 percent, the prices of the state and the city from 1 to 1000 percent, the state buying below what it sells")
	ErrFirmCapital  = errors.New("the storage must hold from 1 hour to 30 days, the capital from 0 to 100 percent")
)

// FirmPlan is what an enterprise costs, as the administrators set it:
// Price votes to found, Upgrade×level votes from its cash to raise the
// level, up to Levels; a level seats Staff workers and costs Upkeep
// hundredths of a vote an hour.
type FirmPlan struct {
	Kind    FirmKind
	Price   int64
	Upgrade int64
	Levels  int
	Staff   int
	Upkeep  int64
}

func (p FirmPlan) check() error {
	if p.Price < 1 || p.Price > MaxAmount || p.Upgrade < 0 || p.Upgrade > MaxAmount ||
		p.Levels < 1 || p.Levels > MaxFirmLevels || p.Staff < 1 || p.Staff > MaxFirmStaff ||
		p.Upkeep < 0 || p.Upkeep > MaxFirmPrice {
		return ErrFirmPlan
	}

	return nil
}

// UpgradeCost is the votes the level after level costs.
func (p FirmPlan) UpgradeCost(level int) int64 {
	return p.Upgrade * int64(level)
}

// Invested is the votes founding and upgrading to level cost.
func (p FirmPlan) Invested(level int) int64 {
	return p.Price + p.Upgrade*int64(level-1)*int64(level)/2
}

// GoodPlan is the price of a good: Base hundredths of a vote a unit, about
// which the state buys and sells it and the city buys it; Norm units a
// day halve what the state and the city pay and raise by half what the
// state asks.
type GoodPlan struct {
	Good Good
	Base int64
	Norm int64
}

func (p GoodPlan) check() error {
	if p.Base < 1 || p.Base > MaxFirmPrice || p.Norm < 1 || p.Norm > MaxGoodNorm {
		return ErrGoodPlan
	}

	return nil
}

// FirmRules are the enterprises of the site: the plans of all kinds and
// goods in order; the standard wage of a worker an hour, in hundredths;
// the fee in percent the exchange takes of a sale of goods and of shares;
// the percents of the base price the state buys (Floor) and sells
// (Ceiling) at and the city pays (Retail); the hours of full work the
// storage holds; and the percent of the price an enterprise starts with
// in its cash.
type FirmRules struct {
	Plans   []FirmPlan
	Goods   []GoodPlan
	Wage    int64
	Fee     int
	Floor   int
	Ceiling int
	Retail  int
	Storage int
	Capital int
}

// NewFirmRules checks rules; ErrFirmPlans, ErrFirmPlan, ErrGoodPlan,
// ErrFirmPercents, ErrFirmCapital.
func NewFirmRules(r FirmRules) (FirmRules, error) {
	if len(r.Plans) != len(FirmKinds) || len(r.Goods) != len(Goods) {
		return FirmRules{}, ErrFirmPlans
	}

	for i, p := range r.Plans {
		if p.Kind != FirmKinds[i] {
			return FirmRules{}, ErrFirmPlans
		}
		if err := p.check(); err != nil {
			return FirmRules{}, err
		}
	}

	for i, g := range r.Goods {
		if g.Good != Goods[i] {
			return FirmRules{}, ErrFirmPlans
		}
		if err := g.check(); err != nil {
			return FirmRules{}, err
		}
	}

	if r.Wage < 1 || r.Wage > MaxFirmPrice || r.Fee < 0 || r.Fee > MaxFirmFee ||
		r.Floor < 1 || r.Floor > MaxFirmPercent || r.Ceiling < 1 || r.Ceiling > MaxFirmPercent ||
		r.Floor >= r.Ceiling || r.Retail < 1 || r.Retail > MaxFirmPercent {
		return FirmRules{}, ErrFirmPercents
	}

	if r.Storage < 1 || r.Storage > MaxFirmStorage || r.Capital < 0 || r.Capital > 100 {
		return FirmRules{}, ErrFirmCapital
	}

	r.Plans = append([]FirmPlan(nil), r.Plans...)
	r.Goods = append([]GoodPlan(nil), r.Goods...)

	return r, nil
}

// DefaultFirmRules are the enterprises until the administrators change
// them. Run alone with the state as the market, a level pays itself back
// in about three days: an extracting enterprise makes about 14 votes an
// hour, a processing one 35, a factory and a shop 80; trading with each
// other pays them more.
func DefaultFirmRules() FirmRules {
	plan := func(kind FirmKind, price, upkeep int64) FirmPlan {
		return FirmPlan{Kind: kind, Price: price, Upgrade: price * 4 / 5, Levels: 10, Staff: 10, Upkeep: upkeep}
	}

	return FirmRules{
		Plans: []FirmPlan{
			plan(FirmFarm, 1_000, 100), plan(FirmMine, 1_000, 100), plan(FirmOilrig, 1_000, 100), plan(FirmForestry, 1_000, 100),
			plan(FirmMill, 2_500, 200), plan(FirmSteelworks, 2_500, 200), plan(FirmChemworks, 2_500, 200), plan(FirmSawmill, 2_500, 200),
			plan(FirmBakery, 6_000, 300), plan(FirmJoinery, 6_000, 300), plan(FirmElectroplant, 6_000, 300), plan(FirmShop, 6_000, 300),
		},
		Goods: []GoodPlan{
			{GoodGrain, 25, 24_000}, {GoodOre, 32, 19_200}, {GoodOil, 42, 14_400}, {GoodWood, 28, 21_600},
			{GoodFlour, 180, 12_000}, {GoodMetal, 225, 9_600}, {GoodPlastic, 300, 7_200}, {GoodBoards, 200, 10_800},
			{GoodBread, 1_000, 6_000}, {GoodFurniture, 2_400, 2_400}, {GoodElectronics, 3_800, 1_920},
		},
		Wage: 50, Fee: 2, Floor: 80, Ceiling: 120, Retail: 150, Storage: 48, Capital: 20,
	}
}

func (r FirmRules) Plan(kind FirmKind) (FirmPlan, error) {
	for _, p := range r.Plans {
		if p.Kind == kind {
			return p, nil
		}
	}

	return FirmPlan{}, ErrUnknownFirm
}

func (r FirmRules) Good(g Good) (GoodPlan, error) {
	for _, p := range r.Goods {
		if p.Good == g {
			return p, nil
		}
	}

	return GoodPlan{}, ErrUnknownGood
}

// Productivity is how much a worker paid wage makes against one paid the
// standard: the square root of the ratio, from half to one and a half.
func (r FirmRules) Productivity(wage int64) float64 {
	return min(max(math.Sqrt(float64(wage)/float64(r.Wage)), 0.5), 1.5)
}

// WageBounds are the wages an enterprise may pay: from half the standard
// to three times it.
func (r FirmRules) WageBounds() (lowest, highest int64) {
	return max(r.Wage/2, 1), r.Wage * 3
}

// A price curve: what the state or the city pays or asks for the units of
// a good bought or sold past sold units today, in hundredths of a vote.
// Paying, the price falls as start×norm/(norm+sold): selling much in a
// day lowers it for everyone; asking, it rises as start×(1+sold/norm).
// The sums are integrals, so that a lot sold at once fetches what it
// would sold a unit at a time.
type curve struct {
	start float64
	norm  float64
	rises bool
}

func (r FirmRules) curve(g Good, percent int, rises bool) (curve, error) {
	p, err := r.Good(g)
	if err != nil {
		return curve{}, err
	}

	return curve{start: float64(p.Base) * float64(percent) / 100, norm: float64(p.Norm), rises: rises}, nil
}

// scaled is the curve with every price times k.
func (c curve) scaled(k float64) curve {
	c.start *= k

	return c
}

// at is the price of the next unit past sold.
func (c curve) at(sold int64) float64 {
	v := float64(sold)
	if c.rises {
		return c.start * (1 + v/c.norm)
	}

	return c.start * c.norm / (c.norm + v)
}

// sum is the price of units past sold.
func (c curve) sum(sold, units int64) float64 {
	v, q := float64(sold), float64(units)
	if c.rises {
		return c.start * (q + (2*v*q+q*q)/(2*c.norm))
	}

	return c.start * c.norm * math.Log((c.norm+v+q)/(c.norm+v))
}

// within is how many units past sold go before the price of the next one
// passes limit: falls below it paying, rises above it asking.
func (c curve) within(sold, limit int64) int64 {
	if limit <= 0 {
		if c.rises {
			return 0
		}

		return math.MaxInt32
	}

	var end float64
	if c.rises {
		end = c.norm * (float64(limit)/c.start - 1)
	} else {
		end = c.start*c.norm/float64(limit) - c.norm
	}

	return max(int64(math.Floor(end))-sold, 0)
}

// StateBuys is the curve of what the state pays for g.
func (r FirmRules) StateBuys(g Good) (curve, error) {
	return r.curve(g, r.Floor, false)
}

// StateSells is the curve of what the state asks for g.
func (r FirmRules) StateSells(g Good) (curve, error) {
	return r.curve(g, r.Ceiling, true)
}

// CityBuys is the curve of what the city pays the shops for g.
func (r FirmRules) CityBuys(g Good) (curve, error) {
	return r.curve(g, r.Retail, false)
}

// Capacity is how many units of g an enterprise of spec at level holds:
// Storage hours of the most its workers may use or give.
func (r FirmRules) Capacity(spec FirmSpec, plan FirmPlan, level int, g Good) int64 {
	perHour := spec.perHour(g) * float64(plan.Staff*level) * 1.5

	return int64(math.Ceil(perHour * float64(r.Storage)))
}
