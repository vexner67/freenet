package user

import (
	"errors"
	"math"
	"time"
)

// BusinessKind is a business a page may own, from the cheapest to the
// dearest; a page owns at most one of each.
type BusinessKind string

const (
	BusinessKiosk   BusinessKind = "kiosk"
	BusinessCafe    BusinessKind = "cafe"
	BusinessCarwash BusinessKind = "carwash"
	BusinessFactory BusinessKind = "factory"
)

// BusinessKinds are all businesses in order.
var BusinessKinds = []BusinessKind{BusinessKiosk, BusinessCafe, BusinessCarwash, BusinessFactory}

// BusinessAction is what an owner does with a business.
type BusinessAction string

const (
	// BusinessOrder buys supplies from a supplier with the votes of the
	// owner; they come in the hours of the supplier.
	BusinessOrder BusinessAction = "order"
	// BusinessContract sets (or, with no units, ends) a standing order of
	// a supply: the buyer orders it from the till when it runs low.
	BusinessContract BusinessAction = "contract"
	// BusinessPrice sets the price of an offer.
	BusinessPrice BusinessAction = "price"
	// BusinessHire takes a candidate on, BusinessFire lets a worker go.
	BusinessHire BusinessAction = "hire"
	BusinessFire BusinessAction = "fire"
	// BusinessBonus pays a worker eight hours of the wage from the votes
	// of the owner: the mood rises.
	BusinessBonus BusinessAction = "bonus"
	// BusinessRaise raises a wage a tenth: the mood rises.
	BusinessRaise BusinessAction = "raise"
	// BusinessUpgrade raises the level.
	BusinessUpgrade BusinessAction = "upgrade"
	// BusinessRepair brings the premises back to full condition and mends
	// a breakdown.
	BusinessRepair BusinessAction = "repair"
	// BusinessAdvertise buys an advertising campaign.
	BusinessAdvertise BusinessAction = "advertise"
	// BusinessCollect takes the cash out of the till.
	BusinessCollect BusinessAction = "collect"
	// BusinessDeposit puts votes into the till: pays a debt off.
	BusinessDeposit BusinessAction = "deposit"
	// BusinessDecide answers the event that waits for the owner.
	BusinessDecide BusinessAction = "decide"
	// BusinessSell sells the business back to the site, or closes a
	// bankrupt one.
	BusinessSell BusinessAction = "sell"
)

var businessActions = []BusinessAction{
	BusinessOrder, BusinessContract, BusinessPrice, BusinessHire, BusinessFire, BusinessBonus, BusinessRaise,
	BusinessUpgrade, BusinessRepair, BusinessAdvertise, BusinessCollect, BusinessDeposit, BusinessDecide, BusinessSell,
}

// FullCondition is the condition of new premises, in hundredths of a
// percent.
const FullCondition = 10_000

// The bounds of the business settings: not rules of the game, only what
// the arithmetic and the fields of the API hold.
const (
	MaxBusinessLevels   = 1_000
	MaxBusinessPercent  = 10_000
	MaxBusinessPrice    = MaxAmount * 100
	MaxBusinessUnits    = 1_000_000_000
	MaxBusinessStaff    = 1_000
	MaxBusinessWear     = FullCondition
	MaxBusinessAdHours  = 365 * 24
	MaxBusinessAdBoost  = 10_000
	MaxBusinessTax      = 100
	maxBusinessSimSteps = 2_000
)

var (
	ErrUnknownBusiness   = errors.New("unknown business")
	ErrUnknownAction     = errors.New("unknown business action")
	ErrBusinessOwned     = errors.New("you own this business already")
	ErrBusinessNotFound  = errors.New("you do not own this business")
	ErrBusinessBankrupt  = errors.New("the business went bankrupt")
	ErrBusinessInDebt    = errors.New("pay the debts of the business off first")
	ErrBusinessTopLevel  = errors.New("the business is at its top level")
	ErrBusinessIntact    = errors.New("the premises need no repair")
	ErrBusinessOrder     = errors.New("an order must be at least 1 unit and fit in the storage with what is on the way")
	ErrBusinessContract  = errors.New("a contract must order at least 1 unit, fit in the storage and order below what it fills")
	ErrBusinessSetPrice  = errors.New("the price must be from 1")
	ErrBusinessDeposit   = errors.New("a deposit must be at least 1 vote")
	ErrNothingToCollect  = errors.New("there is not a whole vote in the till")
	ErrBusinessNotYet    = errors.New("businesses pay pages a week old with a filled profile")
	ErrBusinessPlans     = errors.New("the settings must set every business once, in order")
	ErrBusinessPlanPrice = errors.New("a business must cost at least 1 vote, an upgrade at least 0")
	ErrBusinessLevels    = errors.New("a business must have from 1 level")
	ErrBusinessPercent   = errors.New("the demand, the wages and the supplies must be from 0 to 10000 percent")
	ErrBusinessCapacity  = errors.New("the till and the debt must be from 1, the seats from 1")
	ErrBusinessCosts     = errors.New("the rent and the repair must be from 0, the wear from 0 to 10000")
	ErrBusinessAd        = errors.New("an advertisement must cost from 0, last at least 1 hour and raise the demand at least 1 percent")
	ErrBusinessTax       = errors.New("the tax must be from 0 to 100 percent")
)

func NewBusinessKind(s string) (BusinessKind, error) {
	for _, k := range BusinessKinds {
		if string(k) == s {
			return k, nil
		}
	}

	return "", ErrUnknownBusiness
}

func (k BusinessKind) String() string {
	return string(k)
}

func NewBusinessAction(s string) (BusinessAction, error) {
	for _, a := range businessActions {
		if string(a) == s {
			return a, nil
		}
	}

	return "", ErrUnknownAction
}

// BusinessPlan is how a business works, as the administrators set it;
// what it deals in and whom it employs is BusinessCatalog. Money is in
// votes unless said: Price buys it and Upgrade×level raises its level, up
// to Levels. The customers of its offers are Demand percent of those of
// the catalog; a level seats Staff workers, keeps Till votes in the till,
// pays Rent hundredths an hour and goes bankrupt past Debt votes of debt.
// The premises lose Wear hundredths of a percent of condition an hour,
// and a percent costs Repair to bring back. An advertisement costs
// AdPrice and raises the demand AdBoost percent for AdHours.
type BusinessPlan struct {
	Kind    BusinessKind
	Price   int64
	Upgrade int64
	Levels  int
	Demand  int
	Staff   int
	Rent    int64
	Wear    int
	Repair  int64
	Till    int64
	Debt    int64
	AdPrice int64
	AdHours int
	AdBoost int
}

func (p BusinessPlan) check() error {
	switch {
	case p.Price < 1 || p.Price > MaxAmount || p.Upgrade < 0 || p.Upgrade > MaxAmount:
		return ErrBusinessPlanPrice
	case p.Levels < 1 || p.Levels > MaxBusinessLevels:
		return ErrBusinessLevels
	case p.Demand < 0 || p.Demand > MaxBusinessPercent:
		return ErrBusinessPercent
	case p.Till < 1 || p.Till > MaxAmount || p.Debt < 1 || p.Debt > MaxAmount || p.Staff < 1 || p.Staff > MaxBusinessStaff:
		return ErrBusinessCapacity
	case p.Rent < 0 || p.Rent > MaxBusinessPrice || p.Repair < 0 || p.Repair > MaxAmount || p.Wear < 0 || p.Wear > MaxBusinessWear:
		return ErrBusinessCosts
	case p.AdPrice < 0 || p.AdPrice > MaxAmount || p.AdHours < 1 || p.AdHours > MaxBusinessAdHours ||
		p.AdBoost < 1 || p.AdBoost > MaxBusinessAdBoost:
		return ErrBusinessAd
	}

	return nil
}

// StaffAt is how many workers the level seats.
func (p BusinessPlan) StaffAt(level int) int {
	return p.Staff * level
}

// BusinessRules are the businesses of the site: the plans of all kinds in
// order, the tax percent of the cash taken out (into the treasury), and
// the percents of the wages and the supplies of the catalog all
// businesses pay.
type BusinessRules struct {
	plans []BusinessPlan
	tax   int
	wages int
	goods int
}

func NewBusinessRules(plans []BusinessPlan, tax, wages, goods int) (BusinessRules, error) {
	if len(plans) != len(BusinessKinds) {
		return BusinessRules{}, ErrBusinessPlans
	}

	for i, p := range plans {
		if p.Kind != BusinessKinds[i] {
			return BusinessRules{}, ErrBusinessPlans
		}
		if err := p.check(); err != nil {
			return BusinessRules{}, err
		}
	}

	switch {
	case tax < 0 || tax > MaxBusinessTax:
		return BusinessRules{}, ErrBusinessTax
	case wages < 0 || wages > MaxBusinessPercent || goods < 0 || goods > MaxBusinessPercent:
		return BusinessRules{}, ErrBusinessPercent
	}

	return BusinessRules{plans: append([]BusinessPlan(nil), plans...), tax: tax, wages: wages, goods: goods}, nil
}

// DefaultBusinessRules are the businesses until the administrators change
// them: run well, each pays its price back in a month or so.
func DefaultBusinessRules() BusinessRules {
	return BusinessRules{
		plans: []BusinessPlan{
			{
				Kind: BusinessKiosk, Price: 500, Upgrade: 300, Levels: 10, Demand: 100, Staff: 4,
				Rent: 10, Wear: 10, Repair: 1, Till: 40, Debt: 100, AdPrice: 10, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessCafe, Price: 3_000, Upgrade: 1_800, Levels: 10, Demand: 100, Staff: 6,
				Rent: 50, Wear: 15, Repair: 3, Till: 200, Debt: 600, AdPrice: 60, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessCarwash, Price: 12_000, Upgrade: 7_200, Levels: 10, Demand: 100, Staff: 7,
				Rent: 200, Wear: 20, Repair: 10, Till: 1_000, Debt: 3_000, AdPrice: 240, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessFactory, Price: 50_000, Upgrade: 30_000, Levels: 10, Demand: 100, Staff: 10,
				Rent: 500, Wear: 15, Repair: 40, Till: 3_000, Debt: 10_000, AdPrice: 1_000, AdHours: 24, AdBoost: 50,
			},
		},
		tax:   5,
		wages: 100,
		goods: 100,
	}
}

func (r BusinessRules) Plans() []BusinessPlan {
	return append([]BusinessPlan(nil), r.plans...)
}

// Tax is the percent of the cash taken out that goes to the treasury.
func (r BusinessRules) Tax() int {
	return r.tax
}

// Wages is the percent of the wages of the catalog workers get.
func (r BusinessRules) Wages() int {
	return r.wages
}

// Goods is the percent of the costs of the catalog supplies cost.
func (r BusinessRules) Goods() int {
	return r.goods
}

func (r BusinessRules) Plan(kind BusinessKind) (BusinessPlan, error) {
	for _, p := range r.plans {
		if p.Kind == kind {
			return p, nil
		}
	}

	return BusinessPlan{}, ErrUnknownBusiness
}

// Business is a business of a page as counted at countedAt. Money in the
// till is in hundredths of a vote (below zero — a debt), the supplies are
// in units; both, like the condition and the moods, are fractions the
// hours add up.
type Business struct {
	kind        BusinessKind
	seed        uint64
	level       int
	condition   float64
	cash        float64
	adUntil     time.Time
	reviewUntil time.Time
	countedAt   time.Time
	bankrupt    bool
	invested    int64
	collected   int64
	stock       map[Supply]float64
	prices      map[Offer]int64
	workers     []Worker
	nextID      int
	window      int64
	hired       []int
	orders      []Order
	contracts   []Contract
	event       Event
	broken      Offer
	brokenUntil time.Time
	hours       []BusinessHour
	notes       []Note
}

// NewBusiness is a business just bought: no workers, no supplies, the
// offers at their fair prices. The seed makes its luck: candidates,
// events, deliveries.
func NewBusiness(plan BusinessPlan, seed uint64, now time.Time) (Business, error) {
	spec, err := BusinessCatalog(plan.Kind)
	if err != nil {
		return Business{}, err
	}

	b := Business{
		kind: plan.Kind, seed: seed, level: 1, condition: FullCondition,
		countedAt: now.UTC(), invested: plan.Price, nextID: 1,
		stock: map[Supply]float64{}, prices: map[Offer]int64{},
	}
	for _, o := range spec.Offers {
		b.prices[o.Offer] = o.Fair
	}

	return b, nil
}

// BusinessState is a business as kept.
type BusinessState struct {
	Kind        BusinessKind
	Seed        uint64
	Level       int
	Condition   float64
	Cash        float64
	AdUntil     time.Time
	ReviewUntil time.Time
	CountedAt   time.Time
	Bankrupt    bool
	Invested    int64
	Collected   int64
	Stock       map[Supply]float64
	Prices      map[Offer]int64
	Workers     []Worker
	NextID      int
	Window      int64
	Hired       []int
	Orders      []Order
	Contracts   []Contract
	Event       Event
	Broken      Offer
	BrokenUntil time.Time
	Hours       []BusinessHour
	Notes       []Note
}

// RestoreBusiness keeps what the catalog still knows: supplies, offers
// and professions it no longer has are dropped, offers it has come at
// their fair prices.
func RestoreBusiness(s BusinessState) Business {
	spec, _ := BusinessCatalog(s.Kind)

	b := Business{
		kind: s.Kind, seed: s.Seed, level: max(s.Level, 1), condition: s.Condition, cash: s.Cash,
		adUntil: s.AdUntil.UTC(), reviewUntil: s.ReviewUntil.UTC(), countedAt: s.CountedAt.UTC(),
		bankrupt: s.Bankrupt, invested: s.Invested, collected: s.Collected, nextID: max(s.NextID, 1),
		window: s.Window, hired: append([]int(nil), s.Hired...), event: s.Event,
		brokenUntil: s.BrokenUntil.UTC(), hours: cloneHours(s.Hours), notes: append([]Note(nil), s.Notes...),
		stock: map[Supply]float64{}, prices: map[Offer]int64{},
	}

	for _, sup := range spec.Supplies {
		if v := s.Stock[sup.Supply]; v > 0 {
			b.stock[sup.Supply] = v
		}
	}
	for _, o := range spec.Offers {
		b.prices[o.Offer] = o.Fair
		if p := s.Prices[o.Offer]; p >= 1 && p <= MaxBusinessPrice {
			b.prices[o.Offer] = p
		}
		if o.Offer == s.Broken {
			b.broken = o.Offer
		}
	}
	for _, w := range s.Workers {
		if _, err := spec.Profession(w.Profession); err == nil {
			b.workers = append(b.workers, w)
		}
	}
	for _, o := range s.Orders {
		if _, err := spec.Supply(o.Supply); err == nil {
			b.orders = append(b.orders, o)
		}
	}
	for _, c := range s.Contracts {
		if _, err := spec.Supply(c.Supply); err == nil {
			b.contracts = append(b.contracts, c)
		}
	}

	return b
}

func (b Business) State() BusinessState {
	b = b.clone()

	return BusinessState{
		Kind: b.kind, Seed: b.seed, Level: b.level, Condition: b.condition, Cash: b.cash,
		AdUntil: b.adUntil, ReviewUntil: b.reviewUntil, CountedAt: b.countedAt, Bankrupt: b.bankrupt,
		Invested: b.invested, Collected: b.collected, Stock: b.stock, Prices: b.prices,
		Workers: b.workers, NextID: b.nextID, Window: b.window, Hired: b.hired, Orders: b.orders,
		Contracts: b.contracts, Event: b.event, Broken: b.broken, BrokenUntil: b.brokenUntil,
		Hours: b.hours, Notes: b.notes,
	}
}

// clone is b sharing nothing with it, without the empty supplies, as it
// is restored: actions change the clone, so a business held elsewhere
// (inmem) stays as it was.
func (b Business) clone() Business {
	stock := make(map[Supply]float64, len(b.stock))
	for k, v := range b.stock {
		if v > 0 {
			stock[k] = v
		}
	}

	prices := make(map[Offer]int64, len(b.prices))
	for k, v := range b.prices {
		prices[k] = v
	}

	b.stock, b.prices = stock, prices
	b.workers = append([]Worker(nil), b.workers...)
	b.hired = append([]int(nil), b.hired...)
	b.orders = append([]Order(nil), b.orders...)
	b.contracts = append([]Contract(nil), b.contracts...)
	b.hours = cloneHours(b.hours)
	b.notes = append([]Note(nil), b.notes...)

	return b
}

func (b Business) Kind() BusinessKind {
	return b.kind
}

func (b Business) Level() int {
	return b.level
}

func (b Business) Bankrupt() bool {
	return b.bankrupt
}

// Cash is the till in hundredths of a vote, rounded down; below zero it
// is a debt.
func (b Business) Cash() int64 {
	return int64(math.Floor(b.cash))
}

// Condition is the premises in hundredths of a percent.
func (b Business) Condition() int {
	return int(b.condition)
}

func (b Business) AdUntil() time.Time {
	return b.adUntil
}

// ReviewUntil is until when a good review brings more customers.
func (b Business) ReviewUntil() time.Time {
	return b.reviewUntil
}

// Stock is the units of a supply in the storage.
func (b Business) Stock(s Supply) float64 {
	return b.stock[s]
}

// Price is the price of an offer in hundredths of a vote.
func (b Business) Price(o Offer) int64 {
	return b.prices[o]
}

func (b Business) Workers() []Worker {
	return append([]Worker(nil), b.workers...)
}

func (b Business) Orders() []Order {
	return append([]Order(nil), b.orders...)
}

func (b Business) Contracts() []Contract {
	return append([]Contract(nil), b.contracts...)
}

// Event is what waits for the owner to decide; false without one.
func (b Business) Event() (Event, bool) {
	return b.event, b.event.Kind != ""
}

// Broken is the offer whose equipment broke down, and until when a
// technician mends it (zero — until a repair); "" without a breakdown.
func (b Business) Broken() (Offer, time.Time) {
	return b.broken, b.brokenUntil
}

// Hours are the last hours of the business, the oldest first.
func (b Business) Hours() []BusinessHour {
	return cloneHours(b.hours)
}

// Notes are what happened lately, the oldest first.
func (b Business) Notes() []Note {
	return append([]Note(nil), b.notes...)
}

// TillAt is the most the till keeps, in hundredths of a vote.
func (b Business) TillAt(p BusinessPlan) float64 {
	return float64(p.Till) * float64(b.level) * 100
}

// DebtAt is the debt past which the business goes bankrupt, in
// hundredths of a vote.
func (b Business) DebtAt(p BusinessPlan) float64 {
	return float64(p.Debt) * float64(b.level) * 100
}

// StorageOf is the units of a supply the storage keeps.
func (b Business) StorageOf(s SupplySpec) int64 {
	return s.Storage * int64(b.level)
}

// Seats are the workers the business may have.
func (b Business) Seats(p BusinessPlan) int {
	return p.StaffAt(b.level)
}

// InDebt is whether the till is below zero.
func (b Business) InDebt() bool {
	return b.cash < 0
}

func (b Business) checkOpen() error {
	if b.bankrupt {
		return ErrBusinessBankrupt
	}

	return nil
}

// Priced is b with an offer at price hundredths of a vote.
func (b Business) Priced(o Offer, price int64) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}
	if _, ok := b.prices[o]; !ok {
		return Business{}, ErrUnknownOffer
	}
	if price < 1 || price > MaxBusinessPrice {
		return Business{}, ErrBusinessSetPrice
	}

	b = b.clone()
	b.prices[o] = price

	return b, nil
}

// UpgradeCost is what the next level of b costs.
func (b Business) UpgradeCost(p BusinessPlan) int64 {
	return p.Upgrade * int64(b.level)
}

// RepairCost is what a repair of b costs now: a started percent counts
// whole, a breakdown — a fortieth of the price of the business.
func (b Business) RepairCost(p BusinessPlan) int64 {
	cost := p.Repair * int64(math.Ceil((FullCondition-b.condition)/100))
	if b.broken != "" {
		cost += breakdownCost(p)
	}

	return cost
}

// Upgraded is b a level higher and what that cost.
func (b Business) Upgraded(p BusinessPlan) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}
	if b.level >= p.Levels {
		return Business{}, 0, ErrBusinessTopLevel
	}

	cost := b.UpgradeCost(p)
	b = b.clone()
	b.level++
	b.invested += cost

	return b, cost, nil
}

// Repaired is b in full condition, its breakdown mended, and what that
// cost: a started percent counts whole, a breakdown — a fortieth of the
// price of the business.
func (b Business) Repaired(p BusinessPlan, now time.Time) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}
	if b.condition >= FullCondition && b.broken == "" {
		return Business{}, 0, ErrBusinessIntact
	}

	cost := b.RepairCost(p)
	b = b.clone()
	if b.broken != "" {
		b.note(Note{At: now.UTC(), Kind: NoteRepaired, Subject: string(b.broken)})
		b.broken, b.brokenUntil = "", time.Time{}
		if b.event.Kind == EventBreakdown {
			b.event = Event{}
		}
	}
	b.condition = FullCondition

	return b, cost, nil
}

// Advertised is b advertised for AdHours more from now, or from the end
// of the campaign on now, and what that cost.
func (b Business) Advertised(p BusinessPlan, now time.Time) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}

	from := now.UTC()
	if b.adUntil.After(from) {
		from = b.adUntil
	}

	b = b.clone()
	b.adUntil = from.Add(time.Duration(p.AdHours) * time.Hour)

	return b, p.AdPrice, nil
}

// TaxOf is the tax percent of the cash b takes out: an accountant takes
// his skill off it.
func (b Business) TaxOf(r BusinessRules) int {
	return max(r.Tax()-b.skillOf(ProfAccountant), 0)
}

// Collected is b with the whole votes of its till taken out: what the
// owner gets and the tax of the treasury, rounded up.
func (b Business) Collected(r BusinessRules) (Business, int64, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, 0, err
	}

	votes := int64(math.Floor(b.cash / 100))
	if votes < 1 {
		return Business{}, 0, 0, ErrNothingToCollect
	}

	taxed := (votes*int64(b.TaxOf(r)) + 99) / 100
	b = b.clone()
	b.cash -= float64(votes) * 100
	b.collected += votes - taxed

	return b, votes - taxed, taxed, nil
}

// Deposited is b with votes put into the till.
func (b Business) Deposited(votes int64) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}
	if votes < 1 || votes > MaxAmount {
		return Business{}, ErrBusinessDeposit
	}

	b = b.clone()
	b.cash += float64(votes) * 100

	return b, nil
}

// Resale is what the site pays for b: half of what was put into buying
// and upgrading it and the whole votes of the till; nothing for a
// bankrupt one, which only closes. ErrBusinessInDebt for one in debt.
func (b Business) Resale() (int64, error) {
	if b.bankrupt {
		return 0, nil
	}
	if b.cash < 0 {
		return 0, ErrBusinessInDebt
	}

	return b.invested/2 + int64(b.cash/100), nil
}

// Invested is the votes put into buying and upgrading b.
func (b Business) Invested() int64 {
	return b.invested
}

// CollectedVotes is the votes the owner took out of b, after the tax.
func (b Business) CollectedVotes() int64 {
	return b.collected
}

// CheckBusinessAt is ErrBusinessNotYet unless u may take votes out of a
// business at now: the same pages as take the daily bonus.
func (u *User) CheckBusinessAt(now time.Time) error {
	if err := u.CheckBonusAt(now); err != nil {
		return ErrBusinessNotYet
	}

	return nil
}

// BusinessStats is how the businesses do: how many there are, the votes
// people spent on them and the votes they paid out.
type BusinessStats struct {
	Businesses int64
	Spent      int64
	Paid       int64
}
