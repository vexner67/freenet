package user

import (
	"errors"
	"math"
	"math/big"
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
	// BusinessManage sets the price of the goods and the staff.
	BusinessManage BusinessAction = "manage"
	// BusinessRestock buys goods into the storage.
	BusinessRestock BusinessAction = "restock"
	// BusinessUpgrade raises the level.
	BusinessUpgrade BusinessAction = "upgrade"
	// BusinessRepair brings the premises back to full condition.
	BusinessRepair BusinessAction = "repair"
	// BusinessAdvertise buys an advertising campaign.
	BusinessAdvertise BusinessAction = "advertise"
	// BusinessCollect takes the cash out of the till.
	BusinessCollect BusinessAction = "collect"
	// BusinessDeposit puts votes into the till: pays a debt off.
	BusinessDeposit BusinessAction = "deposit"
	// BusinessSell sells the business back to the site, or closes a
	// bankrupt one.
	BusinessSell BusinessAction = "sell"
)

// FullCondition is the condition of new premises, in hundredths of a
// percent.
const FullCondition = 10_000

// The bounds of the business settings: not rules of the game, only what
// the arithmetic and the fields of the API hold.
const (
	MaxBusinessLevels   = 1_000
	MaxBusinessDemand   = 1_000_000
	MaxBusinessPrice    = MaxAmount * 100
	MaxBusinessStorage  = 1_000_000_000
	MaxBusinessStaff    = 10_000
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
	ErrBusinessStock     = errors.New("the goods must be at least 1 and fit in the storage")
	ErrBusinessSetPrice  = errors.New("the price of the goods must be from 1")
	ErrBusinessSetStaff  = errors.New("the staff must be from 0 to what the level allows")
	ErrBusinessDeposit   = errors.New("a deposit must be at least 1 vote")
	ErrNothingToCollect  = errors.New("there is not a whole vote in the till")
	ErrBusinessNotYet    = errors.New("businesses pay pages a week old with a filled profile")
	ErrBusinessPlans     = errors.New("the settings must set every business once, in order")
	ErrBusinessPlanPrice = errors.New("a business must cost at least 1 vote, an upgrade at least 0")
	ErrBusinessLevels    = errors.New("a business must have from 1 level")
	ErrBusinessDemand    = errors.New("the demand must be from 0, the fair price from 1, the cost of goods from 0")
	ErrBusinessCapacity  = errors.New("the storage, the till and the debt must be from 1, a worker must serve at least 1 customer")
	ErrBusinessCosts     = errors.New("the wages, the rent and the repair must be from 0, the wear from 0 to 10000")
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
	switch a := BusinessAction(s); a {
	case BusinessManage, BusinessRestock, BusinessUpgrade, BusinessRepair, BusinessAdvertise, BusinessCollect, BusinessDeposit, BusinessSell,
		BusinessImprove:
		return a, nil
	default:
		return "", ErrUnknownAction
	}
}

// BusinessPlan is how a business works, as the administrators set it.
// Money is in votes unless said: Price buys it and Upgrade×level raises
// its level, up to Levels. Each level draws Demand customers an hour at
// the Fair price of the goods (hundredths of a vote a unit) — fewer the
// dearer, none at twice of it, — keeps Storage units and Till votes in the
// till, pays Rent hundredths an hour and goes bankrupt past Debt votes of
// debt. A unit of goods costs Cost hundredths; a worker serves Serve
// customers an hour for Wage hundredths, a level employs Staff of them.
// The premises lose Wear hundredths of a percent of condition an hour,
// and a percent costs Repair to bring back. An advertisement costs
// AdPrice and raises the demand AdBoost percent for AdHours.
type BusinessPlan struct {
	Kind    BusinessKind
	Price   int64
	Upgrade int64
	Levels  int
	Demand  int64
	Fair    int64
	Cost    int64
	Storage int64
	Serve   int64
	Staff   int
	Wage    int64
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
	case p.Demand < 0 || p.Demand > MaxBusinessDemand || p.Fair < 1 || p.Fair > MaxBusinessPrice ||
		p.Cost < 0 || p.Cost > MaxBusinessPrice:
		return ErrBusinessDemand
	case p.Storage < 1 || p.Storage > MaxBusinessStorage || p.Till < 1 || p.Till > MaxAmount ||
		p.Debt < 1 || p.Debt > MaxAmount || p.Serve < 1 || p.Serve > MaxBusinessDemand ||
		p.Staff < 1 || p.Staff > MaxBusinessStaff:
		return ErrBusinessCapacity
	case p.Wage < 0 || p.Wage > MaxBusinessPrice || p.Rent < 0 || p.Rent > MaxBusinessPrice ||
		p.Repair < 0 || p.Repair > MaxAmount || p.Wear < 0 || p.Wear > MaxBusinessWear:
		return ErrBusinessCosts
	case p.AdPrice < 0 || p.AdPrice > MaxAmount || p.AdHours < 1 || p.AdHours > MaxBusinessAdHours ||
		p.AdBoost < 1 || p.AdBoost > MaxBusinessAdBoost:
		return ErrBusinessAd
	}

	return nil
}

// BusinessRules are the businesses of the site: the plans of all kinds in
// order and the tax percent of the cash taken out, into the treasury.
type BusinessRules struct {
	plans []BusinessPlan
	tax   int
}

func NewBusinessRules(plans []BusinessPlan, tax int) (BusinessRules, error) {
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

	if tax < 0 || tax > MaxBusinessTax {
		return BusinessRules{}, ErrBusinessTax
	}

	return BusinessRules{plans: append([]BusinessPlan(nil), plans...), tax: tax}, nil
}

// DefaultBusinessRules are the businesses until the administrators change
// them: run well, each pays its price back in a month or so; the next
// level, in about as long times the level.
func DefaultBusinessRules() BusinessRules {
	return BusinessRules{
		plans: []BusinessPlan{
			{
				Kind: BusinessKiosk, Price: 500, Upgrade: 300, Levels: 10,
				Demand: 10, Fair: 20, Cost: 10, Storage: 300, Serve: 12, Staff: 2,
				Wage: 20, Rent: 10, Wear: 10, Repair: 1, Till: 40, Debt: 100,
				AdPrice: 10, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessCafe, Price: 3_000, Upgrade: 1_800, Levels: 10,
				Demand: 20, Fair: 60, Cost: 30, Storage: 600, Serve: 10, Staff: 3,
				Wage: 100, Rent: 50, Wear: 15, Repair: 6, Till: 200, Debt: 600,
				AdPrice: 60, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessCarwash, Price: 12_000, Upgrade: 7_200, Levels: 10,
				Demand: 12, Fair: 350, Cost: 150, Storage: 300, Serve: 4, Staff: 4,
				Wage: 300, Rent: 200, Wear: 20, Repair: 24, Till: 1_000, Debt: 3_000,
				AdPrice: 240, AdHours: 24, AdBoost: 50,
			},
			{
				Kind: BusinessFactory, Price: 50_000, Upgrade: 30_000, Levels: 10,
				Demand: 50, Fair: 300, Cost: 150, Storage: 2_000, Serve: 10, Staff: 6,
				Wage: 400, Rent: 500, Wear: 15, Repair: 100, Till: 3_000, Debt: 10_000,
				AdPrice: 1_000, AdHours: 24, AdBoost: 50,
			},
		},
		tax: 5,
	}
}

func (r BusinessRules) Plans() []BusinessPlan {
	return append([]BusinessPlan(nil), r.plans...)
}

// Tax is the percent of the cash taken out that goes to the treasury.
func (r BusinessRules) Tax() int {
	return r.tax
}

func (r BusinessRules) Plan(kind BusinessKind) (BusinessPlan, error) {
	for _, p := range r.plans {
		if p.Kind == kind {
			return p, nil
		}
	}

	return BusinessPlan{}, ErrUnknownBusiness
}

// Business is a business of a page as counted at CountedAt: its level,
// the goods in its storage, their price, its workers, the condition of
// its premises, the cash of its till in hundredths of a vote (below zero —
// a debt), until when it is advertised, whether it went bankrupt, the
// votes put into buying, upgrading and improving it, the votes taken out
// of it and the tiers of its improvements.
type Business struct {
	kind      BusinessKind
	level     int
	stock     int64
	price     int64
	staff     int
	condition int
	cash      int64
	adUntil   time.Time
	countedAt time.Time
	bankrupt  bool
	invested  int64
	collected int64
	perks     map[BusinessPerk]int
}

// NewBusiness is a business just bought: one worker, goods at the fair
// price, an empty storage and till.
func NewBusiness(plan BusinessPlan, now time.Time) Business {
	return Business{
		kind: plan.Kind, level: 1, price: plan.Fair, staff: 1, condition: FullCondition,
		countedAt: now.UTC(), invested: plan.Price, perks: map[BusinessPerk]int{},
	}
}

// BusinessState is a business as kept.
type BusinessState struct {
	Kind      BusinessKind
	Level     int
	Stock     int64
	Price     int64
	Staff     int
	Condition int
	Cash      int64
	AdUntil   time.Time
	CountedAt time.Time
	Bankrupt  bool
	Invested  int64
	Collected int64
	Perks     map[BusinessPerk]int
}

// RestoreBusiness keeps only the known improvements and their tiers.
func RestoreBusiness(s BusinessState) Business {
	perks := map[BusinessPerk]int{}
	for _, p := range BusinessPerks {
		if tier := s.Perks[p]; tier > 0 {
			perks[p] = min(tier, MaxPerkTier)
		}
	}

	return Business{
		kind: s.Kind, level: s.Level, stock: s.Stock, price: s.Price, staff: s.Staff,
		condition: s.Condition, cash: s.Cash, adUntil: s.AdUntil.UTC(), countedAt: s.CountedAt.UTC(),
		bankrupt: s.Bankrupt, invested: s.Invested, collected: s.Collected, perks: perks,
	}
}

func (b Business) State() BusinessState {
	return BusinessState{
		Kind: b.kind, Level: b.level, Stock: b.stock, Price: b.price, Staff: b.staff,
		Condition: b.condition, Cash: b.cash, AdUntil: b.adUntil, CountedAt: b.countedAt,
		Bankrupt: b.bankrupt, Invested: b.invested, Collected: b.collected, Perks: b.Perks(),
	}
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

// Cash is the till in hundredths of a vote; below zero it is a debt.
func (b Business) Cash() int64 {
	return b.cash
}

// StorageAt is how many units of goods the storage of the level holds.
func (p BusinessPlan) StorageAt(level int) int64 {
	return p.Storage * int64(level)
}

// StaffAt is how many workers the level employs at most.
func (p BusinessPlan) StaffAt(level int) int {
	return p.Staff * level
}

// BusinessHour is what a business does in an hour as it is now:
// customers who come, those served (by the staff, while there are goods),
// the income of their purchases and what the wages and the rent cost, in
// hundredths of a vote.
type BusinessHour struct {
	Customers float64
	Served    float64
	Income    float64
	Costs     float64
}

// hour is the hour of b at t, the goods not counted.
func (b Business) hour(p BusinessPlan, t time.Time) BusinessHour {
	demand := float64(p.Demand) * float64(b.level) * b.perkRate(PerkDecor, 20)
	demand *= max(0, 2-float64(b.price)/(float64(p.Fair)*b.perkRate(PerkBrand, 15)))
	demand *= 0.4 + 0.6*float64(b.condition)/FullCondition
	if t.Before(b.adUntil) {
		demand *= 1 + float64(p.AdBoost)/100
	}

	served := min(demand, float64(b.staff)*float64(p.Serve)*b.perkRate(PerkEquipment, 30))

	return BusinessHour{
		Customers: demand,
		Served:    served,
		Income:    served * float64(b.price),
		Costs:     float64(b.staff)*float64(p.Wage)*b.perkRate(PerkTraining, -15) + float64(p.Rent)*float64(b.level),
	}
}

// Hour is the hour of b now: nobody is served without goods or after a
// bankruptcy.
func (b Business) Hour(p BusinessPlan, now time.Time) BusinessHour {
	h := b.hour(p, now.UTC())
	if b.stock <= 0 || b.bankrupt {
		h.Served, h.Income = 0, 0
	}
	if b.bankrupt {
		h.Costs = 0
	}

	return h
}

// At is b at now: the hours since it was counted, an hour after another —
// customers buy the goods, the wages and the rent are paid from the till,
// the premises wear out. The till keeps no more than it holds, the rest is
// lost; a debt past the bound makes the business bankrupt, and then it
// stops.
func (b Business) At(p BusinessPlan, now time.Time) Business {
	now = now.UTC()
	if b.bankrupt || !now.After(b.countedAt) {
		return b
	}

	step := time.Hour
	if elapsed := now.Sub(b.countedAt); elapsed > step*maxBusinessSimSteps {
		step = elapsed / maxBusinessSimSteps
	}

	var (
		stock     = float64(b.stock)
		cash      = float64(b.cash)
		condition = float64(b.condition)
		till      = float64(p.Till) * float64(b.level) * 100
		debt      = -float64(p.Debt) * float64(b.level) * 100
	)

	for t := b.countedAt; t.Before(now); {
		dt := min(step, now.Sub(t))
		hours := dt.Hours()

		b.condition = int(condition)
		h := b.hour(p, t)
		sold := min(h.Served*hours, stock)

		stock -= sold
		cash = min(cash+sold*float64(b.price)-h.Costs*hours, till)
		condition = max(condition-float64(p.Wear)*b.perkRate(PerkFinish, -25)*hours, 0)
		t = t.Add(dt)

		if cash < debt {
			b.bankrupt = true

			break
		}
	}

	b.stock = int64(stock)
	b.cash = int64(math.Floor(cash))
	b.condition = int(condition)
	b.countedAt = now

	return b
}

func (b Business) checkOpen() error {
	if b.bankrupt {
		return ErrBusinessBankrupt
	}

	return nil
}

// Managed is b with goods at price hundredths of a vote and staff
// workers.
func (b Business) Managed(p BusinessPlan, price int64, staff int) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}

	switch {
	case price < 1 || price > MaxBusinessPrice:
		return Business{}, ErrBusinessSetPrice
	case staff < 0 || staff > p.StaffAt(b.level):
		return Business{}, ErrBusinessSetStaff
	}

	b.price, b.staff = price, staff

	return b, nil
}

// Restocked is b with units of goods more and what they cost: whole votes,
// rounded up.
func (b Business) Restocked(p BusinessPlan, units int64) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}
	if units < 1 || units > b.Storage(p)-b.stock {
		return Business{}, 0, ErrBusinessStock
	}

	cost := new(big.Int).Mul(big.NewInt(units), big.NewInt(p.Cost))
	cost.Add(cost, big.NewInt(99))
	cost.Quo(cost, big.NewInt(100))
	if !cost.IsInt64() || cost.Int64() > MaxAmount {
		return Business{}, 0, ErrNotEnoughVotes
	}

	b.stock += units

	return b, cost.Int64(), nil
}

// Upgraded is b a level higher and what that cost.
func (b Business) Upgraded(p BusinessPlan) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}
	if b.level >= p.Levels {
		return Business{}, 0, ErrBusinessTopLevel
	}

	cost := p.Upgrade * int64(b.level)
	b.level++
	b.invested += cost

	return b, cost, nil
}

// Repaired is b in full condition and what that cost: a started percent
// counts whole.
func (b Business) Repaired(p BusinessPlan) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}
	if b.condition >= FullCondition {
		return Business{}, 0, ErrBusinessIntact
	}

	percents := int64(FullCondition-b.condition+99) / 100
	b.condition = FullCondition

	return b, p.Repair * percents, nil
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

	b.adUntil = from.Add(time.Duration(p.AdHours) * time.Hour)

	return b, p.AdPrice, nil
}

// Collected is b with the whole votes of its till taken out: what the
// owner gets and the tax of the treasury, rounded up.
func (b Business) Collected(tax int) (Business, int64, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, 0, err
	}

	votes := b.cash / 100
	if votes < 1 {
		return Business{}, 0, 0, ErrNothingToCollect
	}

	taxed := (votes*int64(tax) + 99) / 100
	b.cash -= votes * 100
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

	b.cash += votes * 100

	return b, nil
}

// Resale is what the site pays for b: half of what was put into buying,
// upgrading and improving it and the whole votes of the till; nothing for a bankrupt
// one, which only closes. ErrBusinessInDebt for one in debt.
func (b Business) Resale() (int64, error) {
	if b.bankrupt {
		return 0, nil
	}
	if b.cash < 0 {
		return 0, ErrBusinessInDebt
	}

	return b.invested/2 + b.cash/100, nil
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
