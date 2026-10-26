package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

var cityStart = time.Date(2027, 1, 1, 0, 30, 0, 0, time.UTC)

func found(t *testing.T, c *user.City, rules user.FirmRules, owner uuid.UUID, kind user.FirmKind) *user.Firm {
	t.Helper()

	f, err := c.Found(rules, owner, kind, "Тест", cityStart)
	if err != nil {
		t.Fatalf("Found: %v", err)
	}

	return f
}

func TestFirmName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in  string
		err error
	}{
		{"  Рога и копыта ", nil},
		{"", user.ErrFirmName},
		{"   ", user.ErrFirmName},
		{"две\nстроки", user.ErrFirmName},
		{string(make([]rune, 41)), user.ErrFirmName},
		{"Завод 🏭", nil},
	} {
		if _, err := user.NewFirmName(tc.in); !errors.Is(err, tc.err) {
			t.Errorf("NewFirmName(%q) = %v, want %v", tc.in, err, tc.err)
		}
	}
}

func TestFirmRules(t *testing.T) {
	t.Parallel()

	if _, err := user.NewFirmRules(user.DefaultFirmRules()); err != nil {
		t.Fatalf("default rules: %v", err)
	}

	bad := user.DefaultFirmRules()
	bad.Floor = bad.Ceiling
	if _, err := user.NewFirmRules(bad); !errors.Is(err, user.ErrFirmPercents) {
		t.Errorf("floor at ceiling: %v", err)
	}

	bad = user.DefaultFirmRules()
	bad.Plans = bad.Plans[1:]
	if _, err := user.NewFirmRules(bad); !errors.Is(err, user.ErrFirmPlans) {
		t.Errorf("missing plan: %v", err)
	}

	bad = user.DefaultFirmRules()
	bad.Goods[0].Norm = 0
	if _, err := user.NewFirmRules(bad); !errors.Is(err, user.ErrGoodPlan) {
		t.Errorf("no norm: %v", err)
	}
}

// TestPayback runs every enterprise alone a day with the state as its
// market: each must earn its price back in two to six days.
func TestPayback(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	for _, kind := range user.FirmKinds {
		c := user.NewCity(cityStart)
		owner := uuid.New()
		f := found(t, c, rules, owner, kind)
		if c.Tick(rules, cityStart.Add(24*time.Hour)) != 24 {
			t.Fatalf("%s: not 24 hours", kind)
		}

		plan, _ := rules.Plan(kind)
		hourly := f.Profit() / 24 / 100
		days := float64(plan.Price) / float64(hourly) / 24
		t.Logf("%s: %d votes an hour, pays back in %.1f days", kind, hourly, days)
		if hourly <= 0 || days < 2 || days > 6 {
			t.Errorf("%s: %d votes an hour, pays back in %.1f days", kind, hourly, days)
		}
		if f.Cash() <= 0 {
			t.Errorf("%s: no cash after a day", kind)
		}
	}
}

func TestTickCatchesUpADayAtMost(t *testing.T) {
	t.Parallel()

	c := user.NewCity(cityStart)
	if n := c.Tick(user.DefaultFirmRules(), cityStart.Add(100*time.Hour)); n != 24 {
		t.Errorf("counted %d hours, want 24", n)
	}
	if n := c.Tick(user.DefaultFirmRules(), cityStart.Add(100*time.Hour)); n != 0 {
		t.Errorf("counted %d hours again", n)
	}
}

func TestNoCashNoWork(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	rules.Capital = 0
	c := user.NewCity(cityStart)
	f := found(t, c, rules, uuid.New(), user.FirmFarm)
	c.Tick(rules, cityStart.Add(3*time.Hour))

	if f.Stock(user.GoodGrain) != 0 || f.Profit() != 0 || f.Cash() != 0 {
		t.Errorf("a farm with no money worked: stock %d, profit %d", f.Stock(user.GoodGrain), f.Profit())
	}
}

func TestDirectorOnly(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	f := found(t, c, rules, uuid.New(), user.FirmFarm)
	if err := c.Rename(uuid.New(), f.ID(), "Чужое"); !errors.Is(err, user.ErrNotDirector) {
		t.Errorf("rename by another: %v", err)
	}
	if _, err := c.Upgrade(rules, uuid.New(), uuid.New()); !errors.Is(err, user.ErrFirmNotFound) {
		t.Errorf("upgrade of none: %v", err)
	}
}

func TestUpgradeAndStaff(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	f := found(t, c, rules, owner, user.FirmFarm)

	if _, err := c.Upgrade(rules, owner, f.ID()); !errors.Is(err, user.ErrFirmCash) {
		t.Fatalf("upgrade with 200 votes: %v", err)
	}
	if err := c.Deposit(owner, f.ID(), 800); err != nil {
		t.Fatal(err)
	}
	if cost, err := c.Upgrade(rules, owner, f.ID()); err != nil || cost != 80_000 || f.Level() != 2 {
		t.Fatalf("upgrade: %d, %v, level %d", cost, err, f.Level())
	}
	if err := c.Staff(rules, owner, f.ID(), 21, 50); !errors.Is(err, user.ErrFirmStaff) {
		t.Errorf("21 seats at level 2: %v", err)
	}
	if err := c.Staff(rules, owner, f.ID(), 20, 10); !errors.Is(err, user.ErrFirmStaff) {
		t.Errorf("a wage below half: %v", err)
	}
	if err := c.Staff(rules, owner, f.ID(), 20, 100); err != nil || f.Workers() != 20 || f.Wage() != 100 {
		t.Errorf("staff: %v", err)
	}
}

func TestFoundLimit(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	for range user.MaxFirmsFounded {
		found(t, c, rules, owner, user.FirmFarm)
	}
	if _, err := c.Found(rules, owner, user.FirmMine, "Ещё", cityStart); !errors.Is(err, user.ErrTooManyFirms) {
		t.Errorf("one too many: %v", err)
	}
}

func TestExchange(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	farmer, miller := uuid.New(), uuid.New()
	farm := found(t, c, rules, farmer, user.FirmFarm)
	mill := found(t, c, rules, miller, user.FirmMill)

	if _, err := c.BuyFromState(rules, farmer, farm.ID(), user.GoodGrain, 10, cityStart); !errors.Is(err, user.ErrFirmGood) {
		t.Errorf("a farm buying grain: %v", err)
	}

	// The farm has no grain yet to sell.
	sell := user.ExchangeOrder{Good: user.GoodGrain, Price: 22, Units: 300}
	if _, err := c.Place(rules, farmer, farm.ID(), sell, cityStart); !errors.Is(err, user.ErrFirmStock) {
		t.Fatalf("selling none: %v", err)
	}

	c.Tick(rules, cityStart.Add(time.Hour))
	if err := c.SetAuto(farmer, farm.ID(), user.GoodGrain, user.Auto{}); err != nil {
		t.Fatal(err)
	}
	c.Tick(rules, cityStart.Add(5*time.Hour))
	grain := farm.Stock(user.GoodGrain)
	if grain < 300 {
		t.Fatalf("the farm made %d grain", grain)
	}

	deals, err := c.Place(rules, farmer, farm.ID(), sell, cityStart.Add(5*time.Hour))
	if err != nil || len(deals) != 0 {
		t.Fatalf("selling: %v, %v", deals, err)
	}

	cash := mill.Cash()
	buy := user.ExchangeOrder{Good: user.GoodGrain, Buy: true, Price: 25, Units: 200}
	deals, err = c.Place(rules, miller, mill.ID(), buy, cityStart.Add(5*time.Hour))
	if err != nil || len(deals) != 1 || deals[0].Units != 200 || deals[0].Price != 22 {
		t.Fatalf("buying: %v, %v", deals, err)
	}
	if mill.Cash() != cash-200*22 {
		t.Errorf("the mill paid %d, want %d", cash-mill.Cash(), 200*22)
	}
	if c.Last(user.GoodGrain) != 22 {
		t.Errorf("last price %d", c.Last(user.GoodGrain))
	}

	// The rest of the selling order waits; the farm cannot buy its own.
	orders := c.Orders()
	if len(orders) != 1 || orders[0].Units != 100 {
		t.Fatalf("orders left: %+v", orders)
	}
	if _, err = c.Place(rules, farmer, farm.ID(), user.ExchangeOrder{Good: user.GoodGrain, Buy: true, Price: 30, Units: 1}, cityStart); !errors.Is(err, user.ErrFirmGood) {
		t.Errorf("a farm buying: %v", err)
	}

	// Cancelling gives the grain back.
	before := farm.Stock(user.GoodGrain)
	if err = c.Cancel(miller, orders[0].ID); !errors.Is(err, user.ErrOrderNotFound) {
		t.Errorf("cancel by another: %v", err)
	}
	if err = c.Cancel(farmer, orders[0].ID); err != nil || farm.Stock(user.GoodGrain) != before+100 {
		t.Errorf("cancel: %v, stock %d", err, farm.Stock(user.GoodGrain))
	}
}

func TestOwnCrossingOrder(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	mill := found(t, c, rules, owner, user.FirmMill)
	if _, err := c.BuyFromState(rules, owner, mill.ID(), user.GoodGrain, 100, cityStart); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Place(rules, owner, mill.ID(), user.ExchangeOrder{Good: user.GoodGrain, Buy: true, Price: 20, Units: 10}, cityStart); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Place(rules, owner, mill.ID(), user.ExchangeOrder{Good: user.GoodGrain, Price: 19, Units: 10}, cityStart); !errors.Is(err, user.ErrOwnOrder) {
		t.Errorf("crossing own order: %v", err)
	}
}

func TestStatePrices(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	mill := found(t, c, rules, owner, user.FirmMill)

	first, err := c.BuyFromState(rules, owner, mill.ID(), user.GoodGrain, 500, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.BuyFromState(rules, owner, mill.ID(), user.GoodGrain, 500, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if second <= first {
		t.Errorf("the state asked %d, then %d: the price must rise", first, second)
	}

	got, err := c.SellToState(rules, owner, mill.ID(), user.GoodGrain, 1000, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if got >= first+second {
		t.Errorf("sold back for %d what cost %d", got, first+second)
	}
}

func TestShares(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	founder, investor, other := uuid.New(), uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)

	if _, err := c.OfferShares(founder, f.ID(), 101, 10, cityStart); !errors.Is(err, user.ErrShareOffer) {
		t.Errorf("offering more than held: %v", err)
	}
	offer, err := c.OfferShares(founder, f.ID(), 60, 10, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.BuyShares(rules, founder, offer.ID, 1); !errors.Is(err, user.ErrOwnShares) {
		t.Errorf("buying own: %v", err)
	}

	deal, err := c.BuyShares(rules, investor, offer.ID, 40)
	if err != nil || deal.Votes != 400 || deal.Fee != 8 || deal.SellerID != founder {
		t.Fatalf("deal %+v, %v", deal, err)
	}
	if f.DirectorID() != founder {
		t.Errorf("40 of 100 made the investor the director")
	}

	if _, err = c.BuyShares(rules, investor, offer.ID, 20); err != nil {
		t.Fatal(err)
	}
	if f.DirectorID() != investor || f.SharePrice() != 10 {
		t.Errorf("60 of 100: director %v, price %d", f.DirectorID(), f.SharePrice())
	}
	if len(c.Offers()) != 0 {
		t.Errorf("a sold out offer stays")
	}

	// The new director issues shares: their price goes into the cash.
	if _, err = c.IssueShares(founder, f.ID(), 10, 5, cityStart); !errors.Is(err, user.ErrNotDirector) {
		t.Errorf("issue by the founder: %v", err)
	}
	issue, err := c.IssueShares(investor, f.ID(), 100, 5, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	cash := f.Cash()
	if deal, err = c.BuyShares(rules, other, issue.ID, 100); err != nil || deal.SellerID != uuid.Nil || deal.Fee != 0 {
		t.Fatalf("issue deal %+v, %v", deal, err)
	}
	if f.Cash() != cash+500*100 || c.Shares(f.ID()) != 200 {
		t.Errorf("cash %d, shares %d", f.Cash(), c.Shares(f.ID()))
	}
	if f.DirectorID() != other {
		t.Errorf("100 of 200 must make other the director")
	}

	// Dividends by the shares: founder 40, investor 60, other 100.
	payouts, err := c.Dividend(other, f.ID(), 101)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID]int64{founder: 20, investor: 30, other: 50}
	for _, p := range payouts {
		if want[p.UserID] != p.Votes {
			t.Errorf("%v got %d, want %d", p.UserID, p.Votes, want[p.UserID])
		}
	}
	if f.Cash() != cash+500*100-100*100 {
		t.Errorf("the split vote left the cash: %d", f.Cash())
	}
}

func TestLiquidate(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmMill)
	offer, _ := c.OfferShares(founder, f.ID(), 50, 1, cityStart)
	if _, err := c.BuyShares(rules, investor, offer.ID, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Place(rules, founder, f.ID(), user.ExchangeOrder{Good: user.GoodGrain, Buy: true, Price: 10, Units: 100}, cityStart); err != nil {
		t.Fatal(err)
	}

	payouts, err := c.Liquidate(rules, founder, f.ID())
	if err != nil {
		t.Fatal(err)
	}
	// Half of 2 500 and the 500 of the cash, the order's money among it.
	for _, p := range payouts {
		if p.Votes != 875 {
			t.Errorf("%v got %d, want 875", p.UserID, p.Votes)
		}
	}
	if len(c.Firms()) != 0 || len(c.Orders()) != 0 || len(c.HoldingsOf(investor)) != 0 {
		t.Errorf("the enterprise stays")
	}
}

func TestDropHolders(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	founder, investor := uuid.New(), uuid.New()
	f := found(t, c, rules, founder, user.FirmFarm)
	lone := found(t, c, rules, founder, user.FirmMine)
	offer, _ := c.OfferShares(founder, f.ID(), 30, 1, cityStart)
	if _, err := c.BuyShares(rules, investor, offer.ID, 30); err != nil {
		t.Fatal(err)
	}

	c.DropHolders([]uuid.UUID{founder})
	if f.DirectorID() != investor {
		t.Errorf("the director stays gone")
	}
	if _, err := c.Firm(lone.ID()); !errors.Is(err, user.ErrFirmNotFound) {
		t.Errorf("an enterprise nobody holds stays")
	}
}

func TestShopSellsToTheCity(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	shop := found(t, c, rules, owner, user.FirmShop)
	c.Tick(rules, cityStart.Add(3*time.Hour))

	var sold int64
	for _, h := range shop.Hours() {
		sold += h.Sold
	}
	if sold == 0 {
		t.Errorf("the shop sold nothing")
	}

	quotes := c.Quotes(rules, cityStart.Add(3*time.Hour))
	if quotes[8].Good != user.GoodBread || quotes[8].CityPrice >= 1500 {
		t.Errorf("bread in the city: %+v", quotes[8])
	}
}

func TestRestoreCity(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	owner := uuid.New()
	found(t, c, rules, owner, user.FirmBakery)
	c.Tick(rules, cityStart.Add(2*time.Hour))

	again := user.RestoreCity(c.State())
	if len(again.Firms()) != 1 || again.Firms()[0].Cash() != c.Firms()[0].Cash() || !again.Hour().Equal(c.Hour()) {
		t.Errorf("restored differently")
	}
}
