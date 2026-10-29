package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func held(c *user.City, firmID, userID uuid.UUID) int64 {
	for _, h := range c.Holdings(firmID) {
		if h.UserID == userID {
			return h.Shares
		}
	}

	return 0
}

func TestAccount(t *testing.T) {
	t.Parallel()

	c := newCity()
	ann := uuid.New()
	if err := c.Fund(ann, 0); !errors.Is(err, user.ErrFirmVotes) {
		t.Errorf("funding none: %v", err)
	}
	if err := c.Fund(ann, 50); err != nil || c.Account(ann) != 5_000 {
		t.Fatalf("fund: %v, %d", err, c.Account(ann))
	}
	if err := c.Withdraw(ann, 51); !errors.Is(err, user.ErrAccount) {
		t.Errorf("withdrawing more: %v", err)
	}
	if err := c.Withdraw(ann, 50); err != nil || c.Account(ann) != 0 {
		t.Errorf("withdraw: %v, %d", err, c.Account(ann))
	}

	// More than a trillion at once, but not more than an account holds.
	if err := c.Fund(ann, user.MaxMoved); err != nil || c.Account(ann) != user.MaxMoved*100 {
		t.Fatalf("fund the most: %v, %d", err, c.Account(ann))
	}
	if err := c.Fund(ann, user.MaxMoved+1); !errors.Is(err, user.ErrFirmVotes) {
		t.Errorf("funding above the most: %v", err)
	}
	for c.Account(ann) <= user.MaxPayout-user.MaxMoved*100 {
		if err := c.Fund(ann, user.MaxMoved); err != nil {
			t.Fatalf("fund: %v", err)
		}
	}
	if err := c.Fund(ann, user.MaxMoved); !errors.Is(err, user.ErrFirmVotes) {
		t.Errorf("funding a full account: %v", err)
	}
}

// TestBuyNewShares buys of the exchange the new shares an enterprise puts
// up: their price goes into its cash, above what a share is worth.
func TestBuyNewShares(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)

	q := c.Quote(rules, f, cityStart)
	if q.Float != rules.Float || q.Ask <= q.Book || q.Bid >= q.Book || q.Price < q.Bid || q.Price > q.Ask {
		t.Fatalf("quote of a new enterprise: %+v", q)
	}

	if _, err := c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Shares: 10}, cityStart); !errors.Is(err, user.ErrAccount) {
		t.Errorf("buying with no money: %v", err)
	}
	if err := c.Fund(investor, 1_000); err != nil {
		t.Fatal(err)
	}

	cash := f.Cash()
	fill, err := c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Shares: 10}, cityStart)
	if err != nil || fill.Shares != 10 || fill.Money < 10*q.Ask {
		t.Fatalf("buying 10 at any price: %+v, %v", fill, err)
	}
	if f.Cash() != cash+fill.Money || f.Float() != rules.Float-10 || held(c, f.ID(), investor) != 10 {
		t.Errorf("cash %d (+%d), float %d, held %d", f.Cash(), fill.Money, f.Float(), held(c, f.ID(), investor))
	}
	if c.Account(investor) != 100_000-fill.Money {
		t.Errorf("account %d", c.Account(investor))
	}

	// The price rises with the shares the exchange sold in a day.
	if again := c.Quote(rules, f, cityStart); again.Ask <= q.Ask || f.Price() <= q.Price {
		t.Errorf("ask %d after %d, price %d after %d", again.Ask, q.Ask, f.Price(), q.Price)
	}

	// No new shares left, none bought back: the exchange has none to sell.
	if err = c.Issue(rules, investor, f.ID(), 5); !errors.Is(err, user.ErrNotDirector) {
		t.Errorf("issue by an investor: %v", err)
	}
	if err = c.Issue(rules, founder, f.ID(), 0); err != nil {
		t.Fatal(err)
	}
	if q = c.Quote(rules, f, cityStart); q.Ask != 0 {
		t.Errorf("ask %d with nothing to sell", q.Ask)
	}
	if _, err = c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Shares: 1}, cityStart); !errors.Is(err, user.ErrStockNoDeal) {
		t.Errorf("buying of nobody: %v", err)
	}
}

// TestSellToTheExchange sells shares to the exchange: it pays less than
// they are worth closed and sells them again, its price to the treasury.
func TestSellToTheExchange(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)
	if err := c.Issue(rules, founder, f.ID(), 0); err != nil {
		t.Fatal(err)
	}

	if _, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Shares: 101}, cityStart); !errors.Is(err, user.ErrStockShares) {
		t.Errorf("selling more than held: %v", err)
	}

	q := c.Quote(rules, f, cityStart)
	fill, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Shares: 10}, cityStart)
	if err != nil || fill.Shares != 10 || fill.Money > 10*q.Book*95/100 || c.Account(founder) != fill.Money {
		t.Fatalf("selling 10 at any price: %+v, %v, book %d", fill, err, q.Book)
	}
	if held(c, f.ID(), user.MarketID) != 10 || c.Shares(f.ID()) != 100 {
		t.Errorf("the exchange holds %d", held(c, f.ID(), user.MarketID))
	}

	// The exchange sells them again; the cash of the enterprise stays.
	if err = c.Fund(investor, 1_000); err != nil {
		t.Fatal(err)
	}
	cash := f.Cash()
	if fill, err = c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Shares: 15}, cityStart); err != nil || fill.Shares != 10 {
		t.Fatalf("buying of the exchange: %+v, %v", fill, err)
	}
	if f.Cash() != cash || held(c, f.ID(), user.MarketID) != 0 {
		t.Errorf("cash %d, the exchange holds %d", f.Cash(), held(c, f.ID(), user.MarketID))
	}

	// Dividends: the founder holds 90, the investor 10.
	payouts, err := c.Dividend(founder, f.ID(), 100)
	if err != nil || len(payouts) != 2 {
		t.Fatalf("dividend: %+v, %v", payouts, err)
	}
}

func TestStockOrders(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmMill)

	q := c.Quote(rules, f, cityStart)
	high := q.Ask * 2
	if _, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Price: high, Shares: 30}, cityStart); err != nil {
		t.Fatal(err)
	}
	if c.Free(f.ID(), founder) != 70 {
		t.Errorf("free %d of 100 with 30 for sale", c.Free(f.ID(), founder))
	}
	if _, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Buy: true, Price: high, Shares: 1}, cityStart); !errors.Is(err, user.ErrOwnStockOrder) {
		t.Errorf("crossing own order: %v", err)
	}

	// A buying order below the asks waits with its money kept.
	if err := c.Fund(investor, 10_000); err != nil {
		t.Fatal(err)
	}
	low := q.Bid / 2
	if _, err := c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Price: low, Shares: 10}, cityStart); err != nil {
		t.Fatal(err)
	}
	if c.Account(investor) != 1_000_000-low*10 || len(c.StockOrders()) != 2 {
		t.Fatalf("account %d, orders %d", c.Account(investor), len(c.StockOrders()))
	}
	waiting := c.StockOrders()[1]
	if err := c.CancelStock(founder, waiting.ID); !errors.Is(err, user.ErrStockOrderNotFound) {
		t.Errorf("cancel by another: %v", err)
	}
	if err := c.CancelStock(investor, waiting.ID); err != nil || c.Account(investor) != 1_000_000 {
		t.Errorf("cancel: %v, account %d", err, c.Account(investor))
	}

	// Buying above the order of the founder deals at its price, the new
	// shares of the exchange being dearer; the seller pays the fee.
	fill, err := c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Buy: true, Price: high + 100, Shares: 30}, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if fill.Shares != 30 || held(c, f.ID(), investor) != 30 {
		t.Fatalf("fill %+v, held %d", fill, held(c, f.ID(), investor))
	}
	if c.Account(founder) == 0 || c.Account(founder) >= fill.Money {
		t.Errorf("the founder got %d of %d", c.Account(founder), fill.Money)
	}
	if c.Account(investor) != 1_000_000-fill.Money {
		t.Errorf("the investor paid %d, account %d", fill.Money, c.Account(investor))
	}
}

func TestDirectorByShares(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)

	sell(t, c, rules, f, founder, investor, 60)
	if f.DirectorID() != investor {
		t.Errorf("60 of 100 must make the investor the director")
	}

	// The exchange never directs; an enterprise nobody holds closes.
	if _, err := c.PlaceStock(rules, investor, user.StockOrder{FirmID: f.ID(), Shares: 60}, cityStart); err != nil {
		t.Fatal(err)
	}
	if f.DirectorID() != founder {
		t.Errorf("the director is %v, want the founder", f.DirectorID())
	}
	if _, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Shares: 40}, cityStart); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Firm(f.ID()); !errors.Is(err, user.ErrFirmNotFound) {
		t.Errorf("an enterprise only the exchange holds stays")
	}
}

// TestWaitingOrderMeetsTheExchange: a selling order above the bid of the
// exchange is dealt at the end of an hour the bid passed it.
func TestWaitingOrderMeetsTheExchange(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder := uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)

	q := c.Quote(rules, f, cityStart)
	if _, err := c.PlaceStock(rules, founder, user.StockOrder{FirmID: f.ID(), Price: q.Bid * 2, Shares: 5}, cityStart); err != nil {
		t.Fatal(err)
	}
	if err := c.Deposit(founder, f.ID(), 5_000); err != nil {
		t.Fatal(err)
	}
	c.Tick(rules, cityStart.Add(time.Hour))

	if len(c.StockOrders()) != 0 || held(c, f.ID(), user.MarketID) != 5 || c.Account(founder) < q.Bid*2*5 {
		t.Errorf("orders %d, the exchange holds %d, account %d", len(c.StockOrders()), held(c, f.ID(), user.MarketID), c.Account(founder))
	}
}

func TestCandlesAndIndex(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	f := found(t, c, rules, uuid.New(), user.FirmFarm)
	found(t, c, rules, uuid.New(), user.FirmMine)
	c.Tick(rules, cityStart.Add(3*time.Hour))

	candles := f.Candles()
	if len(candles) != 3 || candles[2].Close != f.Price() || candles[1].Open != candles[0].Close {
		t.Fatalf("candles %+v, price %d", candles, f.Price())
	}
	index := c.Index()
	if len(index) != 3 || index[0].Value != 100_000 || index[2].Value <= 0 {
		t.Errorf("index %+v", index)
	}
	// The farm earned in three hours: its price rose with its profit.
	if candles[2].Close <= candles[0].Open {
		t.Errorf("the price went from %d to %d", candles[0].Open, candles[2].Close)
	}
}

func TestRestoreOldOffers(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	founder := uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)

	s := c.State()
	s.Offers = []user.ShareOffer{
		{ID: uuid.New(), FirmID: f.ID(), SellerID: founder, Shares: 30, Price: 12, PlacedAt: cityStart},
		{ID: uuid.New(), FirmID: f.ID(), Shares: 10, Price: 5, PlacedAt: cityStart},
	}
	again := user.RestoreCity(s)

	orders := again.StockOrders()
	if len(orders) != 1 || orders[0].Price != 1_200 || orders[0].Shares != 30 || orders[0].Buy {
		t.Errorf("orders %+v", orders)
	}
	if g, _ := again.Firm(f.ID()); g.Float() != rules.Float+10 {
		t.Errorf("float %d", g.Float())
	}
	if len(again.State().Offers) != 0 {
		t.Errorf("offers are kept again")
	}
}
