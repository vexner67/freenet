package user

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
)

// The stock exchange: people keep money on their accounts there, in
// hundredths of a vote, and buy and sell shares of the enterprises with
// orders, matched with each other's at the price of the older one. The
// exchange itself makes the market: it sells the new shares the
// enterprises put up (their price goes into the cash) and those it bought
// of people, and buys any — at prices of its own, quoted by what an
// enterprise is worth closed, what it earned in a day and the mood of the
// market, the more it sold or bought in a day the worse. It never pays
// more than a share is worth closed: what it gets of the shares it holds —
// dividends, liquidation — goes to the treasury.
//
// So that nobody draws a price as they like, as a real exchange does not
// let them: people deal with each other only within StockBand of the
// price of the exchange, and new shares are put up at one price — the
// money of a director buying the shares of their own enterprise goes back
// into its cash, so such a purchase is no demand and moves no price.

// MarketID is the holder of the shares the exchange bought of people.
var MarketID = uuid.Nil

const (
	// CandleHours are the hours of the price an enterprise keeps.
	CandleHours = 7 * 24
	// MaxStockOrders are the most orders a page keeps on the exchange.
	MaxStockOrders = 20
	// maxMood is how far the mood of the market moves a price.
	maxMood = 0.4
	// StockBand is how far from the price of the exchange the orders of
	// people may stand: further off they are refused.
	StockBand = 0.15
	// indexBase is the index of the exchange at its start, in hundredths
	// of a point.
	indexBase = 100_000
)

var (
	ErrStockOrder         = errors.New("an order must be for 1 share or more at a price from 1 hundredth")
	ErrStockShares        = errors.New("you have not so many shares free of your orders")
	ErrAccount            = errors.New("there is not so much money on your account")
	ErrStockNoDeal        = errors.New("nobody sells or buys the shares at this price now")
	ErrOwnStockOrder      = errors.New("you have an order of your own the other way at this price: cancel it first")
	ErrTooManyStockOrders = errors.New("a page keeps at most 20 orders on the exchange")
	ErrStockOrderNotFound = errors.New("there is no such order of shares")
	ErrStockBand          = errors.New("the price of an order must be within 15% of the price of the exchange")
	ErrIssue              = errors.New("an enterprise may put up from 0 new shares, no more than the site lets it have in all")
)

// Account is the money a page keeps on the exchange, in hundredths.
type Account struct {
	UserID uuid.UUID
	Cash   int64
}

// StockOrder is an order of a page to buy or sell Shares of an enterprise
// at Price hundredths a share. A buying order keeps the money of what is
// left of it, a selling one the shares.
type StockOrder struct {
	ID       uuid.UUID
	FirmID   uuid.UUID
	UserID   uuid.UUID
	Buy      bool
	Price    int64
	Shares   int64
	PlacedAt time.Time
}

// Candle is an hour of the price of a share, in hundredths: at its start,
// the highest and the lowest, at its end, and the shares that changed
// hands.
type Candle struct {
	Hour   time.Time
	Open   int64
	High   int64
	Low    int64
	Close  int64
	Volume int64
}

// IndexPoint is the index of the exchange at the end of an hour, in
// hundredths of a point: how the price of all enterprises together went
// from 1 000 points at the start.
type IndexPoint struct {
	Hour  time.Time
	Value int64
}

// StockHour is the shares of an enterprise the exchange sold to people
// and bought of them in an hour.
type StockHour struct {
	Hour   time.Time
	FirmID uuid.UUID
	Sold   int64
	Bought int64
}

// StockFill is what an order dealt at once: the shares and the money in
// hundredths they went for.
type StockFill struct {
	Shares int64
	Money  int64
}

// StockQuote is an enterprise on the exchange now, in hundredths a share:
// what it is worth closed (Book), with a day of its profit (Fair), the
// price now, what the exchange pays and asks for the next share (Ask 0 —
// it has none to sell), the best orders of people (0 — none), the price a
// day ago, and the shares that changed hands in a day; Shares are held by
// people and the exchange (Pool), Float are new ones for sale. Orders of
// people may stand from Low to High.
type StockQuote struct {
	FirmID  uuid.UUID
	Shares  int64
	Pool    int64
	Float   int64
	Book    int64
	Fair    int64
	Price   int64
	Bid     int64
	Ask     int64
	BestBid int64
	BestAsk int64
	DayAgo  int64
	Volume  int64
	Low     int64
	High    int64
}

// Account is the money userID keeps on the exchange.
func (c *City) Account(userID uuid.UUID) int64 {
	for _, a := range c.accounts {
		if a.UserID == userID {
			return a.Cash
		}
	}

	return 0
}

func (c *City) addCash(userID uuid.UUID, hundredths int64) {
	for i := range c.accounts {
		if c.accounts[i].UserID == userID {
			c.accounts[i].Cash += hundredths
			return
		}
	}

	if hundredths != 0 {
		c.accounts = append(c.accounts, Account{UserID: userID, Cash: hundredths})
	}
}

// Fund puts votes, paid apart, on the account of userID; ErrFirmVotes,
// also when the account would hold more than MaxPayout hundredths.
func (c *City) Fund(userID uuid.UUID, votes int64) error {
	if votes < 1 || votes > MaxMoved || c.Account(userID) > MaxPayout-votes*100 {
		return ErrFirmVotes
	}

	c.addCash(userID, votes*100)

	return nil
}

// Withdraw takes votes off the account of userID, paid to it apart;
// ErrFirmVotes, ErrAccount.
func (c *City) Withdraw(userID uuid.UUID, votes int64) error {
	if votes < 1 || votes > MaxMoved {
		return ErrFirmVotes
	}
	if c.Account(userID) < votes*100 {
		return ErrAccount
	}

	c.addCash(userID, -votes*100)

	return nil
}

// StockOrders are the orders of shares, the oldest first.
func (c *City) StockOrders() []StockOrder {
	return append([]StockOrder(nil), c.stockOrders...)
}

// Index is the index of the exchange hour by hour, the oldest first.
func (c *City) Index() []IndexPoint {
	return append([]IndexPoint(nil), c.index...)
}

// Free is the shares of the enterprise userID holds and does not sell in
// its orders.
func (c *City) Free(firmID, userID uuid.UUID) int64 {
	free := c.held(firmID, userID)
	for _, o := range c.stockOrders {
		if o.FirmID == firmID && o.UserID == userID && !o.Buy {
			free -= o.Shares
		}
	}

	return free
}

// Issue puts up shares new shares of the enterprise for the exchange to
// sell, in place of those it put up before, up to the most shares the
// rules let it have; 0 takes them off. ErrIssue.
func (c *City) Issue(rules FirmRules, userID, firmID uuid.UUID, shares int64) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	if shares < 0 || c.Shares(firmID)+shares > rules.MaxShares {
		return ErrIssue
	}

	f.float = shares

	return nil
}

// market is how the exchange quotes an enterprise at at: what it is worth
// closed and with a day of profit, the price between deals, and the
// curves it buys and sells on.
type market struct {
	book, fair, mid int64
	bid, ask        curve
	bought, sold    int64
	supply          int64
}

func (c *City) marketOf(rules FirmRules, f *Firm, at time.Time) market {
	shares := max(c.Shares(f.id), 1)
	book := c.Worth(rules, f) / shares
	premium := min(max(f.Profit(), 0)/shares, book)
	fair := book + premium
	mid := float64(fair) * (1 + f.mood)

	var sold, bought int64
	from := at.UTC().Truncate(time.Hour).Add(-marketWindow)
	for _, h := range c.stockMarket {
		if h.FirmID == f.id && h.Hour.After(from) {
			sold += h.Sold
			bought += h.Bought
		}
	}

	norm := float64(max(shares/5, 10))
	m := market{
		book: book, fair: fair, mid: max(int64(mid), 1), bought: bought, sold: sold,
		supply: f.float + c.held(f.id, MarketID),
		ask:    curve{start: max(mid, float64(book)) * 1.02, norm: norm, rises: true},
		bid:    curve{start: max(min(mid*0.97, float64(book)*0.95), 0), norm: norm},
	}

	return m
}

// band is the prices people may deal at with each other: StockBand
// around the mid of the exchange.
func (m market) band() (low, high int64) {
	return max(int64(math.Ceil(float64(m.mid)*(1-StockBand))), 1), max(int64(float64(m.mid)*(1+StockBand)), 1)
}

// inBand is whether an order of people stands within the band.
func (m market) inBand(price int64) bool {
	low, high := m.band()

	return price >= low && price <= high
}

// price is the price of a share at the end of an hour: the mid of the
// exchange moved by what it sold and bought in a day.
func (m market) price() int64 {
	norm := m.ask.norm
	pressure := float64(m.sold-m.bought) / (norm + float64(m.sold+m.bought))

	return max(int64(float64(m.mid)*(1+0.5*pressure)), 1)
}

// Quote is the enterprise on the exchange at now.
func (c *City) Quote(rules FirmRules, f *Firm, now time.Time) StockQuote {
	m := c.marketOf(rules, f, now)
	q := StockQuote{
		FirmID: f.id, Shares: c.Shares(f.id), Pool: c.held(f.id, MarketID), Float: f.float,
		Book: m.book, Fair: m.fair, Price: f.price, Bid: int64(m.bid.at(m.bought)),
	}
	q.Low, q.High = m.band()
	if q.Price == 0 {
		q.Price = m.price()
	}
	if m.supply > 0 {
		q.Ask = int64(math.Ceil(m.ask.at(m.sold)))
	}

	for _, o := range c.stockOrders {
		switch {
		case o.FirmID != f.id:
		case o.Buy && o.Price > q.BestBid:
			q.BestBid = o.Price
		case !o.Buy && (q.BestAsk == 0 || o.Price < q.BestAsk):
			q.BestAsk = o.Price
		}
	}

	dayAgo := now.UTC().Truncate(time.Hour).Add(-24 * time.Hour)
	for _, k := range f.candles {
		if k.Hour.After(dayAgo) {
			q.Volume += k.Volume
			if q.DayAgo == 0 {
				q.DayAgo = k.Open
			}
		}
	}

	return q
}

// stockFill is an order being dealt: the money it may spend (a buying
// one) and what it dealt.
type stockFill struct {
	order  *StockOrder
	budget int64
	limit  int64
	StockFill
}

// PlaceStock places an order of userID on the exchange: to buy or sell
// shares at price hundredths a share, or at any price (price 0) — then it
// deals what it can at once and the rest is dropped. What meets the orders
// of others or the prices of the exchange deals at once at their prices,
// the better first; the rest of an order with a price waits. Buying pays
// of the account. ErrStockOrder, ErrStockBand, ErrStockShares, ErrAccount,
// ErrStockNoDeal, ErrOwnStockOrder, ErrTooManyStockOrders.
func (c *City) PlaceStock(rules FirmRules, userID uuid.UUID, o StockOrder, now time.Time) (StockFill, error) {
	f, err := c.Firm(o.FirmID)
	if err != nil {
		return StockFill{}, err
	}

	if err = c.checkStock(userID, o); err != nil {
		return StockFill{}, err
	}
	if o.Price > 0 && !c.marketOf(rules, f, now).inBand(o.Price) {
		return StockFill{}, ErrStockBand
	}

	if o.ID, err = uuid.NewV7(); err != nil {
		return StockFill{}, err
	}
	o.UserID, o.PlacedAt = userID, now.UTC()

	fill := stockFill{order: &o, limit: o.Price}
	switch {
	case o.Buy && o.Price > 0:
		fill.budget = o.Price * o.Shares
		c.addCash(userID, -fill.budget)
	case o.Buy:
		fill.budget = c.Account(userID)
		fill.limit = math.MaxInt64
	case o.Price == 0:
		fill.limit = 1
	}

	c.deal(rules, f, &fill, true, now)

	switch {
	case o.Price == 0 && o.Buy:
		c.addCash(userID, -fill.Money)
	case o.Price > 0 && o.Buy:
		c.addCash(userID, fill.budget-o.Price*o.Shares)
	}

	if o.Price > 0 && o.Shares > 0 {
		c.stockOrders = append(c.stockOrders, o)
	}
	if o.Price == 0 && fill.Shares == 0 {
		return StockFill{}, ErrStockNoDeal
	}

	c.afterDeal(f)

	return fill.StockFill, nil
}

func (c *City) checkStock(userID uuid.UUID, o StockOrder) error {
	if o.Shares < 1 || o.Shares > maxFirmUnits || o.Price < 0 || o.Price > MaxFirmPrice {
		return ErrStockOrder
	}

	count := 0
	for _, other := range c.stockOrders {
		if other.UserID != userID {
			continue
		}

		count++
		if o.Price > 0 && other.FirmID == o.FirmID && other.Buy != o.Buy && crossesStock(o, other) {
			return ErrOwnStockOrder
		}
	}
	if o.Price > 0 && count >= MaxStockOrders {
		return ErrTooManyStockOrders
	}

	switch {
	case !o.Buy && c.Free(o.FirmID, userID) < o.Shares:
		return ErrStockShares
	case o.Buy && o.Price > 0 && c.Account(userID) < o.Price*o.Shares:
		return ErrAccount
	case o.Buy && o.Price == 0 && c.Account(userID) < 1:
		return ErrAccount
	}

	return nil
}

func crossesStock(a, b StockOrder) bool {
	if a.Buy {
		return a.Price >= b.Price
	}

	return a.Price <= b.Price
}

// deal deals the order with the orders of others (people, unless only the
// exchange) and the exchange, the better price first, while it is within
// its limit and, buying, its budget.
func (c *City) deal(rules FirmRules, f *Firm, fill *stockFill, people bool, now time.Time) {
	o := fill.order
	for o.Shares > 0 {
		m := c.marketOf(rules, f, now)
		i := -1
		if people {
			i = c.bestStock(*o, m)
		}

		var mm int64
		if o.Buy && m.supply > 0 {
			mm = int64(math.Ceil(m.ask.at(m.sold)))
		} else if !o.Buy {
			mm = int64(m.bid.at(m.bought))
		}
		mmOK := mm > 0 && ((o.Buy && mm <= fill.limit) || (!o.Buy && mm >= fill.limit))

		switch {
		case i >= 0 && (!mmOK || (o.Buy && c.stockOrders[i].Price <= mm) || (!o.Buy && c.stockOrders[i].Price >= mm)):
			if !c.dealPeople(rules, f, fill, i, now) {
				return
			}
		case mmOK:
			if !c.dealMarket(f, fill, m, i, now) {
				return
			}
		default:
			return
		}
	}
}

// bestStock is the order of another o meets first; -1 for none. An order
// the price of the exchange has left behind out of the band waits.
func (c *City) bestStock(o StockOrder, m market) int {
	best := -1
	for i, other := range c.stockOrders {
		if other.FirmID != o.FirmID || other.Buy == o.Buy || other.UserID == o.UserID || !m.inBand(other.Price) {
			continue
		}
		if o.Price > 0 && !crossesStock(o, other) {
			continue
		}

		if best < 0 || betterStock(other, c.stockOrders[best]) {
			best = i
		}
	}

	return best
}

func betterStock(a, b StockOrder) bool {
	if a.Price != b.Price {
		return (a.Buy && a.Price > b.Price) || (!a.Buy && a.Price < b.Price)
	}
	if !a.PlacedAt.Equal(b.PlacedAt) {
		return a.PlacedAt.Before(b.PlacedAt)
	}

	return bytes.Compare(a.ID[:], b.ID[:]) < 0
}

// dealPeople deals the order with the order i of another at its price;
// whether it dealt any.
func (c *City) dealPeople(rules FirmRules, f *Firm, fill *stockFill, i int, now time.Time) bool {
	o := fill.order
	other := &c.stockOrders[i]
	price := other.Price
	units := min(o.Shares, other.Shares)
	if o.Buy {
		units = min(units, fill.budget/price)
	}
	if units < 1 {
		return false
	}

	value := units * price
	fee := value * int64(rules.Fee) / 100
	buyer, seller := o.UserID, other.UserID
	if !o.Buy {
		buyer, seller = seller, buyer
	}

	c.setHolding(f.id, seller, c.held(f.id, seller)-units)
	c.setHolding(f.id, buyer, c.held(f.id, buyer)+units)
	c.addCash(seller, value-fee)
	c.stats.Fees += fee
	if o.Buy {
		// Selling, the money of the buying order was kept when it was
		// placed.
		fill.budget -= value
	}

	fill.Shares += units
	fill.Money += value
	o.Shares -= units
	if other.Shares -= units; other.Shares == 0 {
		c.stockOrders = slices.Delete(c.stockOrders, i, i+1)
	}
	c.trade(f, now, price, units)

	return true
}

// dealMarket deals the order with the exchange: as many shares as go
// before its price passes the limit, the best order of people (next) or,
// buying, the budget. Whether it dealt any.
func (c *City) dealMarket(f *Firm, fill *stockFill, m market, next int, now time.Time) bool {
	o := fill.order
	limit := fill.limit
	if next >= 0 {
		if o.Buy {
			limit = min(limit, c.stockOrders[next].Price)
		} else {
			limit = max(limit, c.stockOrders[next].Price)
		}
	}

	if o.Buy {
		return c.buyOfMarket(f, fill, m, limit, now)
	}

	return c.sellToMarket(f, fill, m, limit, now)
}

func (c *City) buyOfMarket(f *Firm, fill *stockFill, m market, limit int64, now time.Time) bool {
	if f.float > 0 {
		return c.buyNew(f, fill, m, limit, now)
	}

	o := fill.order
	units := min(o.Shares, m.supply)
	if limit < math.MaxInt64 {
		units = min(units, max(m.ask.within(m.sold, limit), 1))
	}
	cost := func(u int64) int64 { return int64(math.Ceil(m.ask.sum(m.sold, u))) }

	// The most the budget pays for: the cost grows with the shares.
	if units > 0 && cost(units) > fill.budget {
		lo, hi := int64(0), units
		for hi-lo > 1 {
			if mid := (lo + hi) / 2; cost(mid) <= fill.budget {
				lo = mid
			} else {
				hi = mid
			}
		}
		units = lo
	}
	if units < 1 {
		return false
	}

	paid := cost(units)
	c.setHolding(f.id, MarketID, c.held(f.id, MarketID)-units)
	c.setHolding(f.id, o.UserID, c.held(f.id, o.UserID)+units)

	fill.budget -= paid
	fill.Shares += units
	fill.Money += paid
	o.Shares -= units
	c.recordStock(f.id, now, units, 0)
	c.trade(f, now, (paid+units-1)/units, units)

	return true
}

// buyNew buys the new shares an enterprise put up, all at the price the
// exchange asks now: the money goes into the cash of the enterprise, so it
// is no demand — the exchange counts none sold, and the price does not
// climb with them.
func (c *City) buyNew(f *Firm, fill *stockFill, m market, limit int64, now time.Time) bool {
	o := fill.order
	price := int64(math.Ceil(m.ask.at(m.sold)))
	if price > limit {
		return false
	}

	units := min(o.Shares, f.float, fill.budget/price)
	if units < 1 {
		return false
	}

	paid := units * price
	f.cash += paid
	f.float -= units
	c.setHolding(f.id, o.UserID, c.held(f.id, o.UserID)+units)

	fill.budget -= paid
	fill.Shares += units
	fill.Money += paid
	o.Shares -= units
	c.trade(f, now, price, units)

	return true
}

func (c *City) sellToMarket(f *Firm, fill *stockFill, m market, limit int64, now time.Time) bool {
	o := fill.order
	units := min(o.Shares, max(m.bid.within(m.bought, limit), 1))
	got := int64(m.bid.sum(m.bought, units))
	if got < 1 {
		return false
	}

	c.setHolding(f.id, o.UserID, c.held(f.id, o.UserID)-units)
	c.setHolding(f.id, MarketID, c.held(f.id, MarketID)+units)
	c.addCash(o.UserID, got)

	fill.Shares += units
	fill.Money += got
	o.Shares -= units
	c.recordStock(f.id, now, 0, units)
	c.trade(f, now, got/units, units)

	return true
}

func (c *City) recordStock(firmID uuid.UUID, at time.Time, sold, bought int64) {
	at = at.UTC().Truncate(time.Hour)
	for i := range c.stockMarket {
		if h := &c.stockMarket[i]; h.FirmID == firmID && h.Hour.Equal(at) {
			h.Sold += sold
			h.Bought += bought
			return
		}
	}

	c.stockMarket = append(c.stockMarket, StockHour{Hour: at, FirmID: firmID, Sold: sold, Bought: bought})
}

// trade is shares changing hands at price in the hour of at.
func (c *City) trade(f *Firm, at time.Time, price, shares int64) {
	k := f.candle(at, price)
	k.High = max(k.High, price)
	k.Low = min(k.Low, price)
	k.Close = price
	k.Volume += shares
	f.price = price
}

// candle is the candle of the hour of at, opened at the price before or,
// for the first, at price.
func (f *Firm) candle(at time.Time, price int64) *Candle {
	at = at.UTC().Truncate(time.Hour)
	if n := len(f.candles); n > 0 && f.candles[n-1].Hour.Equal(at) {
		return &f.candles[n-1]
	}

	open := f.price
	if open == 0 {
		open = price
	}

	f.candles = append(f.candles, Candle{Hour: at, Open: open, High: open, Low: open, Close: open})
	if over := len(f.candles) - CandleHours; over > 0 {
		f.candles = append([]Candle(nil), f.candles[over:]...)
	}

	return &f.candles[len(f.candles)-1]
}

// afterDeal makes the largest shareholder the director after shares
// changed hands; an enterprise no person holds any more closes, its worth
// to the exchange.
func (c *City) afterDeal(f *Firm) {
	if c.humanShares(f.id) == 0 {
		c.remove(f.id)
		return
	}

	c.elect(f)
}

// humanShares are the shares of the enterprise people hold.
func (c *City) humanShares(firmID uuid.UUID) int64 {
	return c.Shares(firmID) - c.held(firmID, MarketID)
}

// CancelStock takes an order of userID off the exchange; a buying one
// gives its money back to the account. ErrStockOrderNotFound.
func (c *City) CancelStock(userID, orderID uuid.UUID) error {
	i := slices.IndexFunc(c.stockOrders, func(o StockOrder) bool { return o.ID == orderID && o.UserID == userID })
	if i < 0 {
		return ErrStockOrderNotFound
	}

	if o := c.stockOrders[i]; o.Buy {
		c.addCash(userID, o.Price*o.Shares)
	}
	c.stockOrders = slices.Delete(c.stockOrders, i, i+1)

	return nil
}

// exchange is the end of the hour at on the exchange: the mood of every
// enterprise drifts, the waiting orders meet the new prices of the
// exchange, the offers past their day close, the hour of the price closes
// and the index with it.
func (c *City) exchange(rules FirmRules, at time.Time) {
	for _, f := range c.firms {
		noise := (c.luck(at, drawMood, f.id)+c.luck(at, drawMoodTwice, f.id)+c.luck(at, drawMoodThrice, f.id))/1.5 - 1
		f.mood = min(max(f.mood*0.9+0.05*noise, -maxMood), maxMood)
	}

	c.meet(rules, at)
	c.closeTenders(at.Add(time.Hour))

	for _, f := range c.firms {
		price := c.marketOf(rules, f, at).price()
		k := f.candle(at, price)
		k.High = max(k.High, price)
		k.Low = min(k.Low, price)
		k.Close = price
		f.price = price
	}

	end := at.Add(time.Hour)
	c.stockMarket = slices.DeleteFunc(c.stockMarket, func(h StockHour) bool { return !h.Hour.After(end.Add(-marketWindow)) })
	c.indexAt(at)
}

// meet deals the waiting orders, the oldest first, with the exchange at
// its prices of the end of the hour at.
func (c *City) meet(rules FirmRules, at time.Time) {
	for _, o := range c.StockOrders() {
		i := slices.IndexFunc(c.stockOrders, func(w StockOrder) bool { return w.ID == o.ID })
		f, err := c.Firm(o.FirmID)
		if i < 0 || err != nil {
			continue
		}

		w := c.stockOrders[i]
		fill := stockFill{order: &w, limit: w.Price}
		if w.Buy {
			fill.budget = w.Price * w.Shares
		}
		c.deal(rules, f, &fill, false, at)
		if fill.Shares == 0 {
			continue
		}

		i = slices.IndexFunc(c.stockOrders, func(o StockOrder) bool { return o.ID == w.ID })
		if w.Buy {
			c.addCash(w.UserID, fill.budget-w.Price*w.Shares)
		}
		if w.Shares == 0 {
			c.stockOrders = slices.Delete(c.stockOrders, i, i+1)
		} else {
			c.stockOrders[i].Shares = w.Shares
		}
		c.afterDeal(f)
	}
}

// indexAt adds the index of the hour at: the last one times how the
// price of the enterprises there were an hour before changed.
func (c *City) indexAt(at time.Time) {
	value := int64(indexBase)
	if n := len(c.index); n > 0 {
		value = c.index[n-1].Value
	}

	var now, before float64
	for _, f := range c.firms {
		n := len(f.candles)
		if n < 2 || !f.candles[n-1].Hour.Equal(at) || !f.candles[n-2].Hour.Equal(at.Add(-time.Hour)) {
			continue
		}

		shares := float64(c.Shares(f.id))
		now += float64(f.candles[n-1].Close) * shares
		before += float64(f.candles[n-2].Close) * shares
	}
	if before > 0 {
		value = max(int64(float64(value)*now/before), 1)
	}

	c.index = append(c.index, IndexPoint{Hour: at, Value: value})
	if over := len(c.index) - CandleHours; over > 0 {
		c.index = append([]IndexPoint(nil), c.index[over:]...)
	}
}
