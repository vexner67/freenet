package user

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxFirmOrders are the most orders an enterprise keeps on the
	// exchange.
	MaxFirmOrders = 10
	// maxCatchUp are the most hours counted at once after the service was
	// down: those before are lost, as if the city slept.
	maxCatchUp = 24
	// marketWindow is how long what the state and the city bought and sold
	// moves their prices.
	marketWindow = 24 * time.Hour
)

var (
	ErrOrder         = errors.New("an order must be for 1 unit or more at a price from 1 hundredth")
	ErrOwnOrder      = errors.New("the enterprise has an order of its own the other way at this price: cancel it first")
	ErrTooManyOrders = errors.New("an enterprise keeps at most 10 orders on the exchange")
	ErrOrderNotFound = errors.New("there is no such order")
)

// Holding is the shares of an enterprise a page holds.
type Holding struct {
	FirmID uuid.UUID
	UserID uuid.UUID
	Shares int64
}

// ShareOffer is shares of an enterprise for sale at Price votes each, as
// the city kept them before the stock exchange: of a shareholder, or new
// ones of the enterprise, with no seller. A city restored turns them into
// orders of the exchange and shares the enterprise puts up.
type ShareOffer struct {
	ID       uuid.UUID
	FirmID   uuid.UUID
	SellerID uuid.UUID
	Shares   int64
	Price    int64
	PlacedAt time.Time
}

// ExchangeOrder is an order of an enterprise on the exchange: to buy or to
// sell Units of a good at Price hundredths a unit. A buying order keeps
// the money of what is left of it, a selling one the units.
type ExchangeOrder struct {
	ID       uuid.UUID
	FirmID   uuid.UUID
	Good     Good
	Buy      bool
	Price    int64
	Units    int64
	PlacedAt time.Time
}

// MarketHour is what changed hands of a good in an hour: the units the
// state sold and bought, the city bought, and the enterprises traded on
// the exchange, and the hundredths they traded for.
type MarketHour struct {
	Hour        time.Time
	Good        Good
	StateSold   int64
	StateBought int64
	City        int64
	Traded      int64
	Turnover    int64
}

// Payout is the votes a shareholder gets.
type Payout struct {
	UserID uuid.UUID
	Votes  int64
}

// Deal is units of a good that changed hands on the exchange at a price.
type Deal struct {
	Good  Good
	Units int64
	Price int64
}

// CityStats are what the enterprises took in and paid out: the votes
// spent on founding and in their cash and shares, those paid to people,
// and the fees of the exchange in hundredths.
type CityStats struct {
	Spent int64
	Paid  int64
	Fees  int64
}

// City is the economy of the enterprises: the enterprises in the order of
// founding, their shareholders, the orders of the exchange of goods and
// what changed hands lately, the stock exchange — the accounts, the
// orders of shares, what the exchange dealt lately and its index — the
// events and the orders of the city. It counts the hours from hour on,
// the hour going on counted at its end, what happens drawn by its luck.
type City struct {
	hour        time.Time
	firms       []*Firm
	holdings    []Holding
	orders      []ExchangeOrder
	market      []MarketHour
	last        map[Good]int64
	stats       CityStats
	accounts    []Account
	stockOrders []StockOrder
	stockMarket []StockHour
	index       []IndexPoint
	events      []Event
	contracts   []Contract
	luck        Luck
}

// CityState is a city as kept; Offers are what the city kept before the
// stock exchange, read and never written.
type CityState struct {
	Hour        time.Time
	Firms       []FirmState
	Holdings    []Holding
	Orders      []ExchangeOrder
	Offers      []ShareOffer
	Market      []MarketHour
	Last        map[Good]int64
	Stats       CityStats
	Accounts    []Account
	StockOrders []StockOrder
	StockMarket []StockHour
	Index       []IndexPoint
	Events      []Event
	Contracts   []Contract
}

// NewCity is a city with no enterprises yet, counting from the hour of
// now.
func NewCity(now time.Time) *City {
	return &City{hour: now.UTC().Truncate(time.Hour), last: map[Good]int64{}, luck: HashLuck}
}

func RestoreCity(s CityState) *City {
	c := &City{
		hour: s.Hour.UTC(), holdings: append([]Holding(nil), s.Holdings...),
		orders: append([]ExchangeOrder(nil), s.Orders...), market: append([]MarketHour(nil), s.Market...),
		last: map[Good]int64{}, stats: s.Stats, accounts: append([]Account(nil), s.Accounts...),
		stockOrders: append([]StockOrder(nil), s.StockOrders...), stockMarket: append([]StockHour(nil), s.StockMarket...),
		index: append([]IndexPoint(nil), s.Index...), events: append([]Event(nil), s.Events...),
		contracts: append([]Contract(nil), s.Contracts...), luck: HashLuck,
	}
	for _, f := range s.Firms {
		if _, err := FirmCatalog(f.Kind); err == nil {
			c.firms = append(c.firms, restoreFirm(f))
		}
	}
	for g, p := range s.Last {
		c.last[g] = p
	}
	c.restoreOffers(s.Offers)

	return c
}

// restoreOffers turns the offers of shares kept before the stock exchange
// into its orders, at their prices in hundredths, and the new shares of
// the enterprises into those they put up.
func (c *City) restoreOffers(offers []ShareOffer) {
	for _, o := range offers {
		f, err := c.Firm(o.FirmID)
		if err != nil || o.Shares < 1 || o.Price < 1 {
			continue
		}

		if o.SellerID == uuid.Nil {
			f.float += o.Shares
			continue
		}

		if shares := min(o.Shares, c.Free(o.FirmID, o.SellerID)); shares > 0 {
			c.stockOrders = append(c.stockOrders, StockOrder{
				ID: o.ID, FirmID: o.FirmID, UserID: o.SellerID, Price: o.Price * 100, Shares: shares, PlacedAt: o.PlacedAt,
			})
		}
	}
}

// SetLuck sets what the city draws its events and moods of; CalmLuck — none.
func (c *City) SetLuck(luck Luck) {
	c.luck = luck
}

func (c *City) State() CityState {
	s := CityState{
		Hour: c.hour, Holdings: append([]Holding(nil), c.holdings...), Orders: append([]ExchangeOrder(nil), c.orders...),
		Market: append([]MarketHour(nil), c.market...), Last: map[Good]int64{}, Stats: c.stats,
		Accounts: append([]Account(nil), c.accounts...), StockOrders: append([]StockOrder(nil), c.stockOrders...),
		StockMarket: append([]StockHour(nil), c.stockMarket...), Index: append([]IndexPoint(nil), c.index...),
		Events: append([]Event(nil), c.events...), Contracts: append([]Contract(nil), c.contracts...),
	}
	for _, f := range c.firms {
		s.Firms = append(s.Firms, f.state())
	}
	for g, p := range c.last {
		s.Last[g] = p
	}

	return s
}

// Hour is the start of the hour the city counts next.
func (c *City) Hour() time.Time { return c.hour }

func (c *City) Stats() CityStats { return c.stats }

// Count adds the votes people spent on the enterprises and got of them.
func (c *City) Count(spent, paid int64) {
	c.stats.Spent += spent
	c.stats.Paid += paid
}

// Firms are all enterprises in the order of founding.
func (c *City) Firms() []*Firm {
	return append([]*Firm(nil), c.firms...)
}

// Firm is the enterprise id; ErrFirmNotFound.
func (c *City) Firm(id uuid.UUID) (*Firm, error) {
	for _, f := range c.firms {
		if f.id == id {
			return f, nil
		}
	}

	return nil, ErrFirmNotFound
}

// Holdings are the shareholders of the enterprise, the founder first.
func (c *City) Holdings(firmID uuid.UUID) []Holding {
	var out []Holding
	for _, h := range c.holdings {
		if h.FirmID == firmID {
			out = append(out, h)
		}
	}

	return out
}

// HoldingsOf are the shares userID holds.
func (c *City) HoldingsOf(userID uuid.UUID) []Holding {
	var out []Holding
	for _, h := range c.holdings {
		if h.UserID == userID {
			out = append(out, h)
		}
	}

	return out
}

// Holders are all pages that hold shares or keep money or orders on the
// stock exchange.
func (c *City) Holders() []uuid.UUID {
	var ids []uuid.UUID
	add := func(id uuid.UUID) {
		if id != MarketID && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, h := range c.holdings {
		add(h.UserID)
	}
	for _, a := range c.accounts {
		add(a.UserID)
	}
	for _, o := range c.stockOrders {
		add(o.UserID)
	}

	return ids
}

// Shares is how many shares of the enterprise people and the exchange
// hold.
func (c *City) Shares(firmID uuid.UUID) int64 {
	var sum int64
	for _, h := range c.holdings {
		if h.FirmID == firmID {
			sum += h.Shares
		}
	}

	return sum
}

func (c *City) held(firmID, userID uuid.UUID) int64 {
	for _, h := range c.holdings {
		if h.FirmID == firmID && h.UserID == userID {
			return h.Shares
		}
	}

	return 0
}

// Orders are the orders of the exchange, the oldest first.
func (c *City) Orders() []ExchangeOrder {
	return append([]ExchangeOrder(nil), c.orders...)
}

// Last is the price of the last deal in g on the exchange; 0 before any.
func (c *City) Last(g Good) int64 {
	return c.last[g]
}

// directed is the enterprise firmID if userID directs it.
func (c *City) directed(userID, firmID uuid.UUID) (*Firm, error) {
	f, err := c.Firm(firmID)
	if err != nil {
		return nil, err
	}

	if f.directorID != userID {
		return nil, ErrNotDirector
	}

	return f, nil
}

// Found founds an enterprise of kind named name for userID, who holds all
// its shares; the price is paid apart. ErrTooManyFirms, ErrUnknownFirm.
func (c *City) Found(rules FirmRules, userID uuid.UUID, kind FirmKind, name FirmName, now time.Time) (*Firm, error) {
	plan, err := rules.Plan(kind)
	if err != nil {
		return nil, err
	}

	directed := 0
	for _, f := range c.firms {
		if f.directorID == userID {
			directed++
		}
	}
	if directed >= rules.MaxFirms {
		return nil, ErrTooManyFirms
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	spec, _ := FirmCatalog(kind) // the plan is of a known kind
	f := &Firm{
		id: id, kind: kind, name: name, directorID: userID, foundedAt: now.UTC(), level: 1,
		workers: plan.Staff, wage: rules.Wage, cash: plan.Price * int64(rules.Capital),
		stock: map[Good]int64{}, carry: map[Good]float64{}, auto: defaultAuto(spec, rules),
		ranks: map[Improvement]int{}, float: rules.Float,
	}

	c.firms = append(c.firms, f)
	c.holdings = append(c.holdings, Holding{FirmID: id, UserID: userID, Shares: rules.Founding})
	f.price = c.marketOf(rules, f, now).price()

	return f, nil
}

// Rename names the enterprise anew; ErrFirmNotFound, ErrNotDirector.
func (c *City) Rename(userID, firmID uuid.UUID, name FirmName) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	f.name = name

	return nil
}

// Upgrade raises the level, paid from the cash: it seats more workers and
// holds more; ErrFirmTopLevel, ErrFirmCash.
func (c *City) Upgrade(rules FirmRules, userID, firmID uuid.UUID) (int64, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return 0, err
	}

	plan, err := rules.Plan(f.kind)
	if err != nil {
		return 0, err
	}

	if f.level >= plan.Levels {
		return 0, ErrFirmTopLevel
	}

	cost := plan.UpgradeCost(f.level) * 100
	if f.cash < cost {
		return 0, ErrFirmCash
	}

	f.cash -= cost
	f.level++

	return cost, nil
}

// Staff sets how many work and for what wage; ErrFirmStaff.
func (c *City) Staff(rules FirmRules, userID, firmID uuid.UUID, workers int, wage int64) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	plan, err := rules.Plan(f.kind)
	if err != nil {
		return err
	}

	lowest, highest := rules.WageBounds()
	if workers < 0 || workers > plan.Staff*f.level || wage < lowest || wage > highest {
		return ErrFirmStaff
	}

	f.workers, f.wage = workers, wage

	return nil
}

// Deposit puts votes, paid apart, into the cash; ErrFirmVotes.
func (c *City) Deposit(userID, firmID uuid.UUID, votes int64) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	if votes < 1 || votes > MaxAmount {
		return ErrFirmVotes
	}

	f.cash += votes * 100

	return nil
}

// SetAuto sets what the enterprise does with g by itself: it buys only
// what it works with and sells only what it makes or a shop sells;
// ErrFirmGood, ErrFirmAuto.
func (c *City) SetAuto(userID, firmID uuid.UUID, g Good, a Auto) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	spec := f.spec()
	if !spec.Keeps(g) || (a.Buy && !spec.IsInput(g)) || (a.Sell && g != spec.Output && !slices.Contains(spec.Sells, g)) {
		return ErrFirmGood
	}
	if a.BuyMax < 0 || a.BuyMax > MaxFirmPrice || a.SellMin < 0 || a.SellMin > MaxFirmPrice {
		return ErrFirmAuto
	}

	f.auto[g] = a

	return nil
}

// share splits votes among the shareholders by their shares, each down to
// a whole vote; the part of the exchange goes to the treasury, paid but
// to nobody.
func (c *City) share(firmID uuid.UUID, votes int64) ([]Payout, int64) {
	total := c.Shares(firmID)
	if total == 0 {
		return nil, 0
	}

	var payouts []Payout
	var paid int64
	for _, h := range c.Holdings(firmID) {
		// at most MaxAmount×MaxFirmShares: within int64
		v := votes * h.Shares / total
		if v <= 0 {
			continue
		}

		paid += v
		if h.UserID != MarketID {
			payouts = append(payouts, Payout{UserID: h.UserID, Votes: v})
		}
	}

	return payouts, paid
}

// Dividend pays votes of the cash to the shareholders by their shares;
// what does not split into whole votes stays. ErrFirmVotes, ErrFirmCash,
// ErrFirmShortCash.
func (c *City) Dividend(userID, firmID uuid.UUID, votes int64) ([]Payout, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return nil, err
	}

	if votes < 1 || votes > MaxAmount {
		return nil, ErrFirmVotes
	}
	if f.cash < votes*100 {
		return nil, ErrFirmCash
	}

	payouts, paid := c.share(firmID, votes)
	if paid == 0 {
		return nil, ErrFirmShortCash
	}

	f.cash -= paid * 100

	return payouts, nil
}

// Worth is what the enterprise is worth closed, in hundredths: the state
// buys it back for half of what founding, the levels and the
// improvements cost, and its money — the cash and what its orders keep —
// goes too.
func (c *City) Worth(rules FirmRules, f *Firm) int64 {
	plan, err := rules.Plan(f.kind)
	if err != nil {
		return f.cash + c.Escrow(f.id)
	}

	return (plan.Invested(f.level)+f.improved)*50 + f.cash + c.Escrow(f.id)
}

// Escrow is the money the buying orders of the enterprise keep.
func (c *City) Escrow(firmID uuid.UUID) int64 {
	var sum int64
	for _, o := range c.orders {
		if o.FirmID == firmID && o.Buy {
			sum += o.Price * o.Units
		}
	}

	return sum
}

// Liquidate sells the enterprise to the state and pays its worth to the
// shareholders by their shares; its goods are lost.
func (c *City) Liquidate(rules FirmRules, userID, firmID uuid.UUID) ([]Payout, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return nil, err
	}

	payouts, _ := c.share(firmID, c.Worth(rules, f)/100)
	c.remove(firmID)

	return payouts, nil
}

// remove takes the enterprise out with its shares, orders and orders of
// shares — the buying ones give their money back — and the orders of the
// city it took.
func (c *City) remove(firmID uuid.UUID) {
	c.firms = slices.DeleteFunc(c.firms, func(f *Firm) bool { return f.id == firmID })
	c.holdings = slices.DeleteFunc(c.holdings, func(h Holding) bool { return h.FirmID == firmID })
	c.orders = slices.DeleteFunc(c.orders, func(o ExchangeOrder) bool { return o.FirmID == firmID })
	c.contracts = slices.DeleteFunc(c.contracts, func(k Contract) bool { return k.FirmID == firmID })
	c.stockOrders = slices.DeleteFunc(c.stockOrders, func(o StockOrder) bool {
		if o.FirmID != firmID {
			return false
		}

		if o.Buy {
			c.addCash(o.UserID, o.Price*o.Shares)
		}

		return true
	})
}

// DropHolders forgets pages gone for good: their shares, accounts and
// orders of shares. An enterprise no person holds any more closes; one
// that lost its director gets the next largest shareholder.
func (c *City) DropHolders(gone []uuid.UUID) {
	if len(gone) == 0 {
		return
	}

	c.holdings = slices.DeleteFunc(c.holdings, func(h Holding) bool { return slices.Contains(gone, h.UserID) })
	c.accounts = slices.DeleteFunc(c.accounts, func(a Account) bool { return slices.Contains(gone, a.UserID) })
	c.stockOrders = slices.DeleteFunc(c.stockOrders, func(o StockOrder) bool { return slices.Contains(gone, o.UserID) })

	for _, f := range c.Firms() {
		c.afterDeal(f)
	}
}

// elect makes the largest shareholder the director; a tie keeps the
// director, or goes to the one who holds the longest. The exchange never
// directs.
func (c *City) elect(f *Firm) {
	best, most := f.directorID, c.held(f.id, f.directorID)
	if best == MarketID {
		most = 0
	}
	for _, h := range c.Holdings(f.id) {
		if h.UserID != MarketID && h.Shares > most {
			best, most = h.UserID, h.Shares
		}
	}

	f.directorID = best
}

// setHolding sets the shares userID holds; none drops the holding.
func (c *City) setHolding(firmID, userID uuid.UUID, shares int64) {
	for i, h := range c.holdings {
		if h.FirmID == firmID && h.UserID == userID {
			if shares == 0 {
				c.holdings = slices.Delete(c.holdings, i, i+1)
			} else {
				c.holdings[i].Shares = shares
			}

			return
		}
	}

	if shares > 0 {
		c.holdings = append(c.holdings, Holding{FirmID: firmID, UserID: userID, Shares: shares})
	}
}

// Incoming is the units of g the buying orders of the enterprise wait
// for.
func (c *City) Incoming(firmID uuid.UUID, g Good) int64 {
	var sum int64
	for _, o := range c.orders {
		if o.FirmID == firmID && o.Good == g && o.Buy {
			sum += o.Units
		}
	}

	return sum
}

// room is how many more units of g the enterprise may take in.
func (c *City) room(rules FirmRules, f *Firm, g Good) int64 {
	plan, err := rules.Plan(f.kind)
	if err != nil {
		return 0
	}

	return max(f.capacity(rules, plan, g)-f.stock[g]-c.Incoming(f.id, g), 0)
}

// Place puts an order of the enterprise on the exchange; what meets the
// orders of others at their prices changes hands at once, and the rest
// waits. A buying order buys only what the enterprise works with and pays
// its money now, a selling one gives its units now. ErrOrder, ErrFirmGood,
// ErrFirmStorage, ErrFirmCash, ErrFirmStock, ErrOwnOrder,
// ErrTooManyOrders.
func (c *City) Place(rules FirmRules, userID, firmID uuid.UUID, o ExchangeOrder, now time.Time) ([]Deal, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return nil, err
	}

	if o.Price < 1 || o.Price > MaxFirmPrice || o.Units < 1 || o.Units > maxFirmUnits {
		return nil, ErrOrder
	}

	if err = c.checkOrder(rules, f, o); err != nil {
		return nil, err
	}

	if o.Buy {
		f.cash -= o.Price * o.Units
	} else {
		f.stock[o.Good] -= o.Units
	}

	if o.ID, err = uuid.NewV7(); err != nil {
		return nil, err
	}
	o.FirmID, o.PlacedAt = firmID, now.UTC()

	deals := c.match(rules, f, &o, now)
	if o.Units > 0 {
		c.orders = append(c.orders, o)
	}

	return deals, nil
}

func (c *City) checkOrder(rules FirmRules, f *Firm, o ExchangeOrder) error {
	spec := f.spec()
	if !spec.Keeps(o.Good) || (o.Buy && !spec.IsInput(o.Good)) {
		return ErrFirmGood
	}

	count := 0
	for _, other := range c.orders {
		if other.FirmID != f.id {
			continue
		}

		count++
		if other.Good == o.Good && other.Buy != o.Buy && crosses(o, other) {
			return ErrOwnOrder
		}
	}
	if count >= MaxFirmOrders {
		return ErrTooManyOrders
	}

	switch {
	case o.Buy && c.room(rules, f, o.Good) < o.Units:
		return ErrFirmStorage
	case o.Buy && f.cash < o.Price*o.Units:
		return ErrFirmCash
	case !o.Buy && f.stock[o.Good] < o.Units:
		return ErrFirmStock
	}

	return nil
}

// crosses is whether a and b of the other way meet at their prices.
func crosses(a, b ExchangeOrder) bool {
	if a.Buy {
		return a.Price >= b.Price
	}

	return a.Price <= b.Price
}

// match deals o with the orders of the other way it meets, the best
// price first, then the oldest.
func (c *City) match(rules FirmRules, f *Firm, o *ExchangeOrder, now time.Time) []Deal {
	var deals []Deal
	for o.Units > 0 {
		i := c.best(*o)
		if i < 0 {
			break
		}

		other := &c.orders[i]
		units := min(o.Units, other.Units)
		price := other.Price

		otherFirm, err := c.Firm(other.FirmID)
		if err != nil {
			c.orders = slices.Delete(c.orders, i, i+1)
			continue
		}

		if o.Buy {
			f.cash += (o.Price - price) * units
			c.settle(rules, f, otherFirm, o.Good, units, price, now)
		} else {
			c.settle(rules, otherFirm, f, o.Good, units, price, now)
		}

		deals = append(deals, Deal{Good: o.Good, Units: units, Price: price})
		o.Units -= units
		if other.Units -= units; other.Units == 0 {
			c.orders = slices.Delete(c.orders, i, i+1)
		}
	}

	return deals
}

// best is the order o meets first: the lowest selling or the highest
// buying price, then the oldest; -1 for none.
func (c *City) best(o ExchangeOrder) int {
	best := -1
	for i, other := range c.orders {
		if other.Good != o.Good || other.Buy == o.Buy || other.FirmID == o.FirmID || !crosses(o, other) {
			continue
		}

		if best < 0 || better(other, c.orders[best]) {
			best = i
		}
	}

	return best
}

func better(a, b ExchangeOrder) bool {
	if a.Price != b.Price {
		return (a.Buy && a.Price > b.Price) || (!a.Buy && a.Price < b.Price)
	}
	if !a.PlacedAt.Equal(b.PlacedAt) {
		return a.PlacedAt.Before(b.PlacedAt)
	}

	return bytes.Compare(a.ID[:], b.ID[:]) < 0
}

// settle hands units of g at price from seller to buyer, whose money the
// order kept already; the seller pays the fee.
func (c *City) settle(rules FirmRules, buyer, seller *Firm, g Good, units, price int64, now time.Time) {
	value := units * price
	fee := value * int64(rules.Fee) / 100

	buyer.stock[g] += units
	bh := buyer.hour(now)
	bh.Bought += units
	bh.Purchases += value

	seller.cash += value - fee
	sh := seller.hour(now)
	sh.Sold += units
	sh.Income += value
	sh.Fees += fee

	c.record(g, now, func(m *MarketHour) {
		m.Traded += units
		m.Turnover += value
	})
	c.last[g] = price
	c.stats.Fees += fee
}

// Cancel takes an order of an enterprise userID directs off the exchange
// and gives back what it keeps; ErrOrderNotFound.
func (c *City) Cancel(userID, orderID uuid.UUID) error {
	i := slices.IndexFunc(c.orders, func(o ExchangeOrder) bool { return o.ID == orderID })
	if i < 0 {
		return ErrOrderNotFound
	}

	o := c.orders[i]
	f, err := c.directed(userID, o.FirmID)
	if err != nil {
		return ErrOrderNotFound
	}

	if o.Buy {
		f.cash += o.Price * o.Units
	} else {
		f.stock[o.Good] += o.Units
	}

	c.orders = slices.Delete(c.orders, i, i+1)

	return nil
}

// record adds to what changed hands of g in the hour of at.
func (c *City) record(g Good, at time.Time, add func(*MarketHour)) {
	at = at.UTC().Truncate(time.Hour)
	for i := range c.market {
		if c.market[i].Good == g && c.market[i].Hour.Equal(at) {
			add(&c.market[i])
			return
		}
	}

	m := MarketHour{Hour: at, Good: g}
	add(&m)
	c.market = append(c.market, m)
}

// volume is what pick counts of g in the day up to the hour of at.
func (c *City) volume(g Good, at time.Time, pick func(MarketHour) int64) int64 {
	at = at.UTC().Truncate(time.Hour)
	var sum int64
	for _, m := range c.market {
		if m.Good == g && m.Hour.After(at.Add(-marketWindow)) && !m.Hour.After(at) {
			sum += pick(m)
		}
	}

	return sum
}

func stateSold(m MarketHour) int64   { return m.StateSold }
func stateBought(m MarketHour) int64 { return m.StateBought }
func cityBought(m MarketHour) int64  { return m.City }
func traded(m MarketHour) int64      { return m.Traded }

// Quote is a good on the market now, in hundredths a unit: what the state
// pays and asks for the next unit, what the city pays a shop, the best
// orders of the exchange (0 — none), the last deal and the units traded
// in a day.
type Quote struct {
	Good      Good
	StateBid  int64
	StateAsk  int64
	CityPrice int64
	BestBid   int64
	BestAsk   int64
	Last      int64
	Volume    int64
}

// Quotes are all goods on the market at now.
func (c *City) Quotes(rules FirmRules, now time.Time) []Quote {
	quotes := make([]Quote, 0, len(Goods))
	for _, g := range Goods {
		q := Quote{Good: g, Last: c.last[g], Volume: c.volume(g, now, traded)}
		if bid, err := rules.StateBuys(g); err == nil {
			q.StateBid = int64(bid.at(c.volume(g, now, stateBought)))
		}
		if ask, err := rules.StateSells(g); err == nil {
			q.StateAsk = int64(math.Ceil(ask.at(c.volume(g, now, stateSold))))
		}
		if city, err := rules.CityBuys(g); err == nil {
			_, events := c.effects(FirmShop, now)
			q.CityPrice = int64(city.scaled(events).at(c.volume(g, now, cityBought)))
		}
		for _, o := range c.orders {
			switch {
			case o.Good != g:
			case o.Buy && o.Price > q.BestBid:
				q.BestBid = o.Price
			case !o.Buy && (q.BestAsk == 0 || o.Price < q.BestAsk):
				q.BestAsk = o.Price
			}
		}

		quotes = append(quotes, q)
	}

	return quotes
}

// BuyFromState buys units of g from the state for the enterprise at once;
// ErrFirmGood, ErrFirmUnits, ErrFirmStorage, ErrFirmCash.
func (c *City) BuyFromState(rules FirmRules, userID, firmID uuid.UUID, g Good, units int64, now time.Time) (int64, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return 0, err
	}

	if !f.spec().IsInput(g) {
		return 0, ErrFirmGood
	}
	if units < 1 || units > maxFirmUnits {
		return 0, ErrFirmUnits
	}
	if c.room(rules, f, g) < units {
		return 0, ErrFirmStorage
	}

	ask, err := rules.StateSells(g)
	if err != nil {
		return 0, err
	}

	cost := int64(math.Ceil(ask.sum(c.volume(g, now, stateSold), units)))
	if cost > f.cash {
		return 0, ErrFirmCash
	}

	c.buyFromState(f, g, units, cost, now)

	return cost, nil
}

func (c *City) buyFromState(f *Firm, g Good, units, cost int64, now time.Time) {
	f.cash -= cost
	f.stock[g] += units
	h := f.hour(now)
	h.Bought += units
	h.Purchases += cost
	c.record(g, now, func(m *MarketHour) { m.StateSold += units })
}

// SellToState sells units of g of the enterprise to the state at once, at
// the prices of the state its sales add to; ErrFirmGood, ErrFirmUnits,
// ErrFirmStock, ErrFirmNoDeal.
func (c *City) SellToState(rules FirmRules, userID, firmID uuid.UUID, g Good, units int64, now time.Time) (int64, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return 0, err
	}

	if !f.spec().Keeps(g) {
		return 0, ErrFirmGood
	}
	if units < 1 || units > maxFirmUnits {
		return 0, ErrFirmUnits
	}
	if f.stock[g] < units {
		return 0, ErrFirmStock
	}

	cv, err := rules.StateBuys(g)
	if err != nil {
		return 0, err
	}

	got := c.sellTo(cv.scaled(f.sales()), f, g, units, now, stateBought, func(m *MarketHour) { m.StateBought += units })
	if got == 0 {
		return 0, ErrFirmNoDeal
	}

	return got, nil
}

// sellTo sells units of g of the enterprise on the curve, past what pick
// counted today; a sale that brings less than a hundredth does not
// happen.
func (c *City) sellTo(cv curve, f *Firm, g Good, units int64, now time.Time, pick func(MarketHour) int64, add func(*MarketHour)) int64 {
	got := int64(cv.sum(c.volume(g, now, pick), units))
	if got < 1 {
		return 0
	}

	f.stock[g] -= units
	f.cash += got
	h := f.hour(now)
	h.Sold += units
	h.Income += got
	c.record(g, now, add)

	return got
}

// Tick counts the hours that have ended by now; how many it counted.
func (c *City) Tick(rules FirmRules, now time.Time) int {
	end := now.UTC().Truncate(time.Hour)
	if end.Sub(c.hour) > maxCatchUp*time.Hour {
		c.hour = end.Add(-maxCatchUp * time.Hour)
	}

	counted := 0
	for c.hour.Before(end) {
		c.count(rules, c.hour)
		c.hour = c.hour.Add(time.Hour)
		counted++
	}

	if counted > 0 {
		c.market = slices.DeleteFunc(c.market, func(m MarketHour) bool { return !m.Hour.After(end.Add(-marketWindow)) })
	}

	return counted
}

// count is the hour at: the orders of the city that ended close, what
// happens is drawn and the city orders goods, every enterprise works it —
// each hour another one first, as the first to sell gets the best prices
// — and the exchange closes it.
func (c *City) count(rules FirmRules, at time.Time) {
	c.expire(at.Add(time.Hour))
	c.happen(rules, at)
	c.order(rules, at)

	if n := len(c.firms); n > 0 {
		first := int(at.Unix()/3600) % n
		for i := range n {
			c.work(rules, c.firms[(first+i)%n], at)
		}
	}

	c.exchange(rules, at)
}

// work is an hour of an enterprise: it pays the wages and the upkeep — as
// much of them as its cash holds, and works as much less; on strike it
// pays the upkeep only — buys what the hour takes, makes or, a shop,
// sells to the city, and sells what it made. Its output is as the wage,
// the wear of the machines, their rank and the events of its kind make
// it, and the machines wear by the hour.
func (c *City) work(rules FirmRules, f *Firm, at time.Time) {
	plan, err := rules.Plan(f.kind)
	if err != nil {
		return
	}

	spec := f.spec()
	h := f.hour(at)

	working := f.workers
	if at.Before(f.idleUntil) {
		working = 0
	}

	wages := int64(working) * f.wage
	upkeep := plan.Upkeep * int64(f.level)
	paid := 1.0
	if need := wages + upkeep; need > f.cash {
		paid = float64(f.cash) / float64(need)
	}

	payWages := min(int64(float64(wages)*paid), f.cash)
	payUpkeep := min(int64(float64(upkeep)*paid), f.cash-payWages)
	f.cash -= payWages + payUpkeep
	h.Wages += payWages
	h.Upkeep += payUpkeep

	output, city := c.effects(f.kind, at)
	workers := float64(working) * paid * rules.Productivity(f.wage) * f.condition() * f.machines() * output
	f.wearOut(float64(working)*paid, plan.Staff*f.level)

	for _, g := range spec.Uses() {
		if spec.IsInput(g) && f.auto[g].Buy {
			c.restock(rules, f, g, int64(math.Ceil(workers*spec.perHour(g)))-f.stock[g], at)
		}
	}

	if spec.IsShop() {
		for _, g := range spec.Sells {
			c.retail(rules, f, g, workers, city, at)
		}

		return
	}

	c.make(rules, f, plan, workers, at)

	if a := f.auto[spec.Output]; a.Sell {
		if cv, err := rules.StateBuys(spec.Output); err == nil {
			c.autoSell(cv.scaled(f.sales()), f, spec.Output, f.stock[spec.Output], a.SellMin, at, stateBought,
				func(m *MarketHour, u int64) { m.StateBought += u })
		}
	}
}

// capacity is how many units of g the storage of the enterprise holds, as
// its level and the rank of the storage make it.
func (f *Firm) capacity(rules FirmRules, plan FirmPlan, g Good) int64 {
	return int64(float64(rules.Capacity(f.spec(), plan, f.level, g)) * f.storage())
}

// Capacity is how many units of g the storage of the enterprise holds.
func (f *Firm) Capacity(rules FirmRules, g Good) int64 {
	plan, err := rules.Plan(f.kind)
	if err != nil {
		return 0
	}

	return f.capacity(rules, plan, g)
}

// restock buys up to want units of g from the state, as far as the
// storage, the cash and the price limit go.
func (c *City) restock(rules FirmRules, f *Firm, g Good, want int64, at time.Time) {
	ask, err := rules.StateSells(g)
	if err != nil {
		return
	}

	sold := c.volume(g, at, stateSold)
	units := min(want, c.room(rules, f, g), ask.within(sold, f.auto[g].BuyMax))
	cost := func(u int64) int64 { return int64(math.Ceil(ask.sum(sold, u))) }

	// The most the cash pays for: the cost grows with the units.
	if units > 0 && cost(units) > f.cash {
		lo, hi := int64(0), units
		for hi-lo > 1 {
			if mid := (lo + hi) / 2; cost(mid) <= f.cash {
				lo = mid
			} else {
				hi = mid
			}
		}
		units = lo
	}

	if units > 0 {
		c.buyFromState(f, g, units, cost(units), at)
	}
}

// make makes the output of the hour of what is in the storage and as the
// storage holds.
func (c *City) make(rules FirmRules, f *Firm, plan FirmPlan, workers float64, at time.Time) {
	spec := f.spec()
	out := spec.Output

	could := workers*spec.Rate[out] + f.carry[out]
	limit := f.capacity(rules, plan, out) - f.stock[out]
	for _, in := range spec.Inputs {
		limit = min(limit, f.stock[in.Good]/in.Units)
	}
	limit = max(limit, 0)

	made := int64(could)
	if made <= limit {
		f.carry[out] = could - float64(made)
	} else {
		made, f.carry[out] = limit, 0
	}

	for _, in := range spec.Inputs {
		f.stock[in.Good] -= made * in.Units
	}
	f.stock[out] += made
	f.hour(at).Made += made
}

// retail is an hour of a shop selling g to the city, at the prices of the
// city its sales and the events (city) make.
func (c *City) retail(rules FirmRules, f *Firm, g Good, workers, city float64, at time.Time) {
	could := workers*f.spec().Rate[g] + f.carry[g]
	served := int64(could)
	f.carry[g] = could - float64(served)

	a := f.auto[g]
	if !a.Sell {
		return
	}

	cv, err := rules.CityBuys(g)
	if err != nil {
		return
	}

	c.autoSell(cv.scaled(f.sales()*city), f, g, min(served, f.stock[g]), a.SellMin, at, cityBought,
		func(m *MarketHour, u int64) { m.City += u })
}

// autoSell sells up to units of g the enterprise has on the curve, while
// the price is not below least.
func (c *City) autoSell(cv curve, f *Firm, g Good, units, least int64, at time.Time,
	pick func(MarketHour) int64, add func(*MarketHour, int64),
) {
	units = min(units, cv.within(c.volume(g, at, pick), least))
	if units <= 0 {
		return
	}

	c.sellTo(cv, f, g, units, at, pick, func(m *MarketHour) { add(m, units) })
}
