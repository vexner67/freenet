package user

import "errors"

// Supply is what a business buys from suppliers and keeps in its storage:
// the goods of a kiosk, the beans and the milk of a cafe.
type Supply string

// Offer is what a business sells: the customers come for it, a worker of
// its profession serves them, and each sale uses its recipe of supplies.
type Offer string

// Profession is the work of a worker: the professions of a kind serve
// its offers, the shared ones help any business.
type Profession string

// Supplier is whom a business buys supplies from: dearer is faster and
// surer.
type Supplier string

const (
	SupplyDrinks    Supply = "drinks"
	SupplySnacks    Supply = "snacks"
	SupplyIcecream  Supply = "icecream"
	SupplyPapers    Supply = "papers"
	SupplyBeans     Supply = "beans"
	SupplyMilk      Supply = "milk"
	SupplyGroceries Supply = "groceries"
	SupplyPastry    Supply = "pastry"
	SupplyShampoo   Supply = "shampoo"
	SupplyWax       Supply = "wax"
	SupplyChemicals Supply = "chemicals"
	SupplyMetal     Supply = "metal"
	SupplyPlastic   Supply = "plastic"
)

const (
	OfferDrinks   Offer = "drinks"
	OfferSnacks   Offer = "snacks"
	OfferIcecream Offer = "icecream"
	OfferPapers   Offer = "papers"
	OfferCoffee   Offer = "coffee"
	OfferLunch    Offer = "lunch"
	OfferPastry   Offer = "pastry"
	OfferWash     Offer = "wash"
	OfferFull     Offer = "full"
	OfferInterior Offer = "interior"
	OfferParts    Offer = "parts"
	OfferCases    Offer = "cases"
	OfferAssembly Offer = "assembly"
)

const (
	ProfSeller  Profession = "seller"
	ProfBarista Profession = "barista"
	ProfCook    Profession = "cook"
	ProfCashier Profession = "cashier"
	ProfWasher  Profession = "washer"
	ProfCleaner Profession = "cleaner"
	ProfWorker  Profession = "worker"
	ProfMaster  Profession = "master"

	// ProfStorekeeper takes the deliveries in: without one a tenth of
	// each is lost.
	ProfStorekeeper Profession = "storekeeper"
	// ProfBuyer runs the contracts with the suppliers and bargains the
	// supplies down.
	ProfBuyer Profession = "buyer"
	// ProfTechnician keeps the premises from wearing and mends the
	// breakdowns himself.
	ProfTechnician Profession = "technician"
	// ProfMarketer brings more customers.
	ProfMarketer Profession = "marketer"
	// ProfAccountant lowers the tax and the fines.
	ProfAccountant Profession = "accountant"
	// ProfGuard keeps the till from thieves.
	ProfGuard Profession = "guard"
)

// SharedProfessions help any business, in order; a business counts only
// its most skilled worker of each.
var SharedProfessions = []Profession{ProfStorekeeper, ProfBuyer, ProfTechnician, ProfMarketer, ProfAccountant, ProfGuard}

const (
	// SupplierWholesale is cheap and slow and may bring less than ordered.
	SupplierWholesale Supplier = "wholesale"
	SupplierReliable  Supplier = "reliable"
	SupplierExpress   Supplier = "express"
)

// Suppliers are all suppliers, from the cheapest.
var Suppliers = []Supplier{SupplierWholesale, SupplierReliable, SupplierExpress}

var (
	ErrUnknownSupply     = errors.New("unknown supply")
	ErrUnknownOffer      = errors.New("unknown offer")
	ErrUnknownSupplier   = errors.New("unknown supplier")
	ErrUnknownProfession = errors.New("unknown profession")
)

func (s Supply) String() string     { return string(s) }
func (o Offer) String() string      { return string(o) }
func (p Profession) String() string { return string(p) }
func (s Supplier) String() string   { return string(s) }

// SupplySpec is a supply of a kind: a unit costs Cost hundredths of a
// vote at the reliable supplier, it spoils in Spoil hours (0 — never) and
// the storage keeps Storage units of it a level.
type SupplySpec struct {
	Supply  Supply
	Cost    int64
	Spoil   int
	Storage int64
}

// Ingredient is the units of a supply a sale uses.
type Ingredient struct {
	Supply Supply
	Units  int64
}

// OfferSpec is an offer of a kind: a worker of Profession serves it, a
// sale uses Recipe and pays the price, and a level draws Demand customers
// an hour at the Fair price (hundredths of a vote) — fewer the dearer,
// none at twice of it.
type OfferSpec struct {
	Offer      Offer
	Profession Profession
	Recipe     []Ingredient
	Fair       int64
	Demand     float64
}

// ProfessionSpec is a profession of a kind: its Wage an hour in
// hundredths of a vote at skill 2, and the customers a worker of it
// serves an hour at skill 2 (0 for the shared professions).
type ProfessionSpec struct {
	Profession Profession
	Wage       int64
	Serve      float64
}

// KindSpec is what a kind of business deals in: its supplies, offers and
// professions, the shared ones last.
type KindSpec struct {
	Kind        BusinessKind
	Supplies    []SupplySpec
	Offers      []OfferSpec
	Professions []ProfessionSpec
}

// SupplierSpec is a supplier: it charges Percent of the cost, brings an
// order in Hours, and may bring up to Short percent less.
type SupplierSpec struct {
	Supplier Supplier
	Percent  int64
	Hours    float64
	Short    int
}

var supplierSpecs = []SupplierSpec{
	{Supplier: SupplierWholesale, Percent: 80, Hours: 6, Short: 15},
	{Supplier: SupplierReliable, Percent: 100, Hours: 2},
	{Supplier: SupplierExpress, Percent: 140, Hours: 0.5},
}

func shared(wage int64) []ProfessionSpec {
	specs := make([]ProfessionSpec, 0, len(SharedProfessions))
	for _, p := range SharedProfessions {
		specs = append(specs, ProfessionSpec{Profession: p, Wage: wage})
	}

	return specs
}

func one(s Supply) []Ingredient {
	return []Ingredient{{Supply: s, Units: 1}}
}

var kindSpecs = []KindSpec{
	{
		Kind: BusinessKiosk,
		Supplies: []SupplySpec{
			{Supply: SupplyDrinks, Cost: 4, Storage: 200},
			{Supply: SupplySnacks, Cost: 3, Storage: 200},
			{Supply: SupplyIcecream, Cost: 4, Storage: 100},
			{Supply: SupplyPapers, Cost: 1, Spoil: 24, Storage: 80},
		},
		Offers: []OfferSpec{
			{Offer: OfferDrinks, Profession: ProfSeller, Recipe: one(SupplyDrinks), Fair: 18, Demand: 8},
			{Offer: OfferSnacks, Profession: ProfSeller, Recipe: one(SupplySnacks), Fair: 14, Demand: 8},
			{Offer: OfferIcecream, Profession: ProfSeller, Recipe: one(SupplyIcecream), Fair: 18, Demand: 4},
			{Offer: OfferPapers, Profession: ProfSeller, Recipe: one(SupplyPapers), Fair: 8, Demand: 4},
		},
		Professions: append([]ProfessionSpec{
			{Profession: ProfSeller, Wage: 25, Serve: 12},
		}, shared(12)...),
	},
	{
		Kind: BusinessCafe,
		Supplies: []SupplySpec{
			{Supply: SupplyBeans, Cost: 8, Storage: 200},
			{Supply: SupplyMilk, Cost: 3, Spoil: 72, Storage: 200},
			{Supply: SupplyGroceries, Cost: 20, Spoil: 72, Storage: 100},
			{Supply: SupplyPastry, Cost: 15, Spoil: 24, Storage: 80},
		},
		Offers: []OfferSpec{
			{Offer: OfferCoffee, Profession: ProfBarista, Recipe: []Ingredient{{SupplyBeans, 1}, {SupplyMilk, 1}}, Fair: 60, Demand: 10},
			{Offer: OfferLunch, Profession: ProfCook, Recipe: one(SupplyGroceries), Fair: 135, Demand: 5},
			{Offer: OfferPastry, Profession: ProfCashier, Recipe: one(SupplyPastry), Fair: 50, Demand: 6},
		},
		Professions: append([]ProfessionSpec{
			{Profession: ProfBarista, Wage: 60, Serve: 10},
			{Profession: ProfCook, Wage: 80, Serve: 4},
			{Profession: ProfCashier, Wage: 40, Serve: 12},
		}, shared(40)...),
	},
	{
		Kind: BusinessCarwash,
		Supplies: []SupplySpec{
			{Supply: SupplyShampoo, Cost: 30, Storage: 100},
			{Supply: SupplyWax, Cost: 60, Storage: 50},
			{Supply: SupplyChemicals, Cost: 80, Storage: 40},
		},
		Offers: []OfferSpec{
			{Offer: OfferWash, Profession: ProfWasher, Recipe: one(SupplyShampoo), Fair: 300, Demand: 6},
			{Offer: OfferFull, Profession: ProfWasher, Recipe: []Ingredient{{SupplyShampoo, 1}, {SupplyWax, 1}}, Fair: 600, Demand: 3},
			{Offer: OfferInterior, Profession: ProfCleaner, Recipe: one(SupplyChemicals), Fair: 675, Demand: 2},
		},
		Professions: append([]ProfessionSpec{
			{Profession: ProfWasher, Wage: 150, Serve: 3},
			{Profession: ProfCleaner, Wage: 150, Serve: 2},
		}, shared(100)...),
	},
	{
		Kind: BusinessFactory,
		Supplies: []SupplySpec{
			{Supply: SupplyMetal, Cost: 70, Storage: 300},
			{Supply: SupplyPlastic, Cost: 40, Storage: 200},
		},
		Offers: []OfferSpec{
			{Offer: OfferParts, Profession: ProfWorker, Recipe: []Ingredient{{SupplyMetal, 2}}, Fair: 600, Demand: 12},
			{Offer: OfferCases, Profession: ProfWorker, Recipe: []Ingredient{{SupplyPlastic, 2}, {SupplyMetal, 1}}, Fair: 750, Demand: 10},
			{Offer: OfferAssembly, Profession: ProfMaster, Recipe: []Ingredient{{SupplyMetal, 3}, {SupplyPlastic, 2}}, Fair: 1_800, Demand: 5},
		},
		Professions: append([]ProfessionSpec{
			{Profession: ProfWorker, Wage: 200, Serve: 6},
			{Profession: ProfMaster, Wage: 400, Serve: 3},
		}, shared(250)...),
	},
}

// BusinessCatalog is what kind deals in.
func BusinessCatalog(kind BusinessKind) (KindSpec, error) {
	for _, k := range kindSpecs {
		if k.Kind == kind {
			return k, nil
		}
	}

	return KindSpec{}, ErrUnknownBusiness
}

func (k KindSpec) Supply(s Supply) (SupplySpec, error) {
	for _, spec := range k.Supplies {
		if spec.Supply == s {
			return spec, nil
		}
	}

	return SupplySpec{}, ErrUnknownSupply
}

func (k KindSpec) Offer(o Offer) (OfferSpec, error) {
	for _, spec := range k.Offers {
		if spec.Offer == o {
			return spec, nil
		}
	}

	return OfferSpec{}, ErrUnknownOffer
}

func (k KindSpec) Profession(p Profession) (ProfessionSpec, error) {
	for _, spec := range k.Professions {
		if spec.Profession == p {
			return spec, nil
		}
	}

	return ProfessionSpec{}, ErrUnknownProfession
}

// Serving are the professions of the kind that serve its offers.
func (k KindSpec) Serving() []Profession {
	var serving []Profession
	for _, spec := range k.Professions {
		if spec.Serve > 0 {
			serving = append(serving, spec.Profession)
		}
	}

	return serving
}

func SupplierCatalog(s Supplier) (SupplierSpec, error) {
	for _, spec := range supplierSpecs {
		if spec.Supplier == s {
			return spec, nil
		}
	}

	return SupplierSpec{}, ErrUnknownSupplier
}

// SupplierSpecs are all suppliers, from the cheapest.
func SupplierSpecs() []SupplierSpec {
	return append([]SupplierSpec(nil), supplierSpecs...)
}
