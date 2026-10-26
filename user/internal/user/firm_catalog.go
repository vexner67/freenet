package user

import "errors"

// Good is what the firms make, trade and sell: raw materials, what is
// made of them, and the goods the city buys.
type Good string

const (
	GoodGrain       Good = "grain"
	GoodOre         Good = "ore"
	GoodOil         Good = "oil"
	GoodWood        Good = "wood"
	GoodFlour       Good = "flour"
	GoodMetal       Good = "metal"
	GoodPlastic     Good = "plastic"
	GoodBoards      Good = "boards"
	GoodBread       Good = "bread"
	GoodFurniture   Good = "furniture"
	GoodElectronics Good = "electronics"
)

// Goods are all goods, from the raw to the finished.
var Goods = []Good{
	GoodGrain, GoodOre, GoodOil, GoodWood, GoodFlour, GoodMetal, GoodPlastic, GoodBoards,
	GoodBread, GoodFurniture, GoodElectronics,
}

// FirmKind is an enterprise: an extracting one makes a raw material, a
// processing one makes a material of a raw one, a factory makes goods of
// the city, and a shop sells them to the city.
type FirmKind string

const (
	FirmFarm         FirmKind = "farm"
	FirmMine         FirmKind = "mine"
	FirmOilrig       FirmKind = "oilrig"
	FirmForestry     FirmKind = "forestry"
	FirmMill         FirmKind = "mill"
	FirmSteelworks   FirmKind = "steelworks"
	FirmChemworks    FirmKind = "chemworks"
	FirmSawmill      FirmKind = "sawmill"
	FirmBakery       FirmKind = "bakery"
	FirmJoinery      FirmKind = "joinery"
	FirmElectroplant FirmKind = "electroplant"
	FirmShop         FirmKind = "shop"
)

// FirmKinds are all enterprises along the chains.
var FirmKinds = []FirmKind{
	FirmFarm, FirmMine, FirmOilrig, FirmForestry, FirmMill, FirmSteelworks, FirmChemworks, FirmSawmill,
	FirmBakery, FirmJoinery, FirmElectroplant, FirmShop,
}

var (
	ErrUnknownFirm = errors.New("unknown enterprise")
	ErrUnknownGood = errors.New("unknown good")
)

func NewGood(s string) (Good, error) {
	for _, g := range Goods {
		if string(g) == s {
			return g, nil
		}
	}

	return "", ErrUnknownGood
}

func (g Good) String() string {
	return string(g)
}

func NewFirmKind(s string) (FirmKind, error) {
	for _, k := range FirmKinds {
		if string(k) == s {
			return k, nil
		}
	}

	return "", ErrUnknownFirm
}

func (k FirmKind) String() string {
	return string(k)
}

// Input is what a unit of the output takes.
type Input struct {
	Good  Good
	Units int64
}

// FirmSpec is what an enterprise does, the same whatever the settings: it
// makes Rate units of Output a worker an hour of Inputs; a shop instead
// sells Rate units of each of Sells a worker an hour.
type FirmSpec struct {
	Kind   FirmKind
	Tier   int
	Output Good
	Inputs []Input
	Sells  []Good
	Rate   map[Good]float64
}

var firmSpecs = []FirmSpec{
	{Kind: FirmFarm, Tier: 1, Output: GoodGrain, Rate: map[Good]float64{GoodGrain: 10}},
	{Kind: FirmMine, Tier: 1, Output: GoodOre, Rate: map[Good]float64{GoodOre: 8}},
	{Kind: FirmOilrig, Tier: 1, Output: GoodOil, Rate: map[Good]float64{GoodOil: 6}},
	{Kind: FirmForestry, Tier: 1, Output: GoodWood, Rate: map[Good]float64{GoodWood: 9}},
	{Kind: FirmMill, Tier: 2, Output: GoodFlour, Inputs: []Input{{GoodGrain, 2}}, Rate: map[Good]float64{GoodFlour: 5}},
	{Kind: FirmSteelworks, Tier: 2, Output: GoodMetal, Inputs: []Input{{GoodOre, 2}}, Rate: map[Good]float64{GoodMetal: 4}},
	{Kind: FirmChemworks, Tier: 2, Output: GoodPlastic, Inputs: []Input{{GoodOil, 2}}, Rate: map[Good]float64{GoodPlastic: 3}},
	{Kind: FirmSawmill, Tier: 2, Output: GoodBoards, Inputs: []Input{{GoodWood, 2}}, Rate: map[Good]float64{GoodBoards: 4.5}},
	{Kind: FirmBakery, Tier: 3, Output: GoodBread, Inputs: []Input{{GoodFlour, 2}}, Rate: map[Good]float64{GoodBread: 2.5}},
	{
		Kind: FirmJoinery, Tier: 3, Output: GoodFurniture, Inputs: []Input{{GoodBoards, 3}, {GoodMetal, 1}},
		Rate: map[Good]float64{GoodFurniture: 1},
	},
	{
		Kind: FirmElectroplant, Tier: 3, Output: GoodElectronics, Inputs: []Input{{GoodMetal, 3}, {GoodPlastic, 3}},
		Rate: map[Good]float64{GoodElectronics: 0.8},
	},
	{
		Kind: FirmShop, Tier: 4, Sells: []Good{GoodBread, GoodFurniture, GoodElectronics},
		Rate: map[Good]float64{GoodBread: 2, GoodFurniture: 0.2, GoodElectronics: 0.12},
	},
}

// FirmCatalog is what kind does.
func FirmCatalog(kind FirmKind) (FirmSpec, error) {
	for _, s := range firmSpecs {
		if s.Kind == kind {
			return s, nil
		}
	}

	return FirmSpec{}, ErrUnknownFirm
}

// FirmSpecs are all enterprises.
func FirmSpecs() []FirmSpec {
	return append([]FirmSpec(nil), firmSpecs...)
}

// Uses are the goods the enterprise keeps in its storage: what it makes or
// sells and what it makes it of.
func (s FirmSpec) Uses() []Good {
	var goods []Good
	if s.Output != "" {
		goods = append(goods, s.Output)
	}
	for _, in := range s.Inputs {
		goods = append(goods, in.Good)
	}

	return append(goods, s.Sells...)
}

// Keeps is whether the enterprise keeps g.
func (s FirmSpec) Keeps(g Good) bool {
	for _, u := range s.Uses() {
		if u == g {
			return true
		}
	}

	return false
}

// IsInput is whether the enterprise buys g to work: what it makes of, or
// what a shop sells.
func (s FirmSpec) IsInput(g Good) bool {
	for _, in := range s.Inputs {
		if in.Good == g {
			return true
		}
	}
	for _, sold := range s.Sells {
		if sold == g {
			return true
		}
	}

	return false
}

// IsShop is whether the enterprise sells to the city instead of making.
func (s FirmSpec) IsShop() bool {
	return len(s.Sells) > 0
}

// perHour is how many units of g the enterprise takes or gives in an hour
// of a worker: its rate for what it makes or sells, the recipe times the
// rate for what it is made of.
func (s FirmSpec) perHour(g Good) float64 {
	if r, ok := s.Rate[g]; ok {
		return r
	}
	for _, in := range s.Inputs {
		if in.Good == g {
			return float64(in.Units) * s.Rate[s.Output]
		}
	}

	return 0
}
