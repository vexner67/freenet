package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestWearAndRepair(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner := uuid.New()
	f := found(t, c, rules, owner, user.FirmFarm)

	if _, err := c.Repair(rules, owner, f.ID()); !errors.Is(err, user.ErrFirmWorn) {
		t.Errorf("repairing new machines: %v", err)
	}

	c.Tick(rules, cityStart.Add(10*time.Hour))
	if f.Wear() < 3.9 || f.Wear() > 4.1 {
		t.Fatalf("ten hours of a full staff wore %.2f", f.Wear())
	}

	plan, _ := rules.Plan(user.FirmFarm)
	cost := user.RepairCost(plan, f)
	if cost != 400 {
		t.Errorf("repairing 4 points of 1 000 votes costs %d hundredths", cost)
	}
	if _, err := c.Repair(rules, uuid.New(), f.ID()); !errors.Is(err, user.ErrNotDirector) {
		t.Errorf("repair by another: %v", err)
	}
	cash := f.Cash()
	if got, err := c.Repair(rules, owner, f.ID()); err != nil || got != cost || f.Wear() != 0 || f.Cash() != cash-cost {
		t.Errorf("repair: %d, %v, wear %.2f", got, err, f.Wear())
	}
}

// TestWornMachines: machines worn past a third make less.
func TestWornMachines(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	made := func(hours int) int64 {
		c := newCity()
		owner := uuid.New()
		f := found(t, c, rules, owner, user.FirmFarm)
		if err := c.Deposit(owner, f.ID(), 10_000); err != nil {
			t.Fatal(err)
		}
		for h := 24; h <= hours; h += 24 {
			c.Tick(rules, cityStart.Add(time.Duration(h)*time.Hour))
		}
		c.Tick(rules, cityStart.Add(time.Duration(hours+1)*time.Hour))

		hs := f.Hours()
		return hs[len(hs)-1].Made
	}

	if fresh, worn := made(0), made(200); worn >= fresh {
		t.Errorf("an hour made %d new and %d worn by 200 hours", fresh, worn)
	}
}

func TestImprove(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner := uuid.New()
	f := found(t, c, rules, owner, user.FirmFarm)
	plain := found(t, c, rules, uuid.New(), user.FirmFarm)

	if _, err := c.Improve(rules, owner, f.ID(), user.ImproveMachines); !errors.Is(err, user.ErrFirmCash) {
		t.Errorf("improving with 200 votes: %v", err)
	}
	if err := c.Deposit(owner, f.ID(), 10_000); err != nil {
		t.Fatal(err)
	}

	worth := c.Worth(rules, f)
	for rank := range user.MaxRank {
		cost, err := c.Improve(rules, owner, f.ID(), user.ImproveMachines)
		if err != nil || cost != int64(rank+1)*500*100 {
			t.Fatalf("rank %d: %d, %v", rank+1, cost, err)
		}
	}
	if _, err := c.Improve(rules, owner, f.ID(), user.ImproveMachines); !errors.Is(err, user.ErrImprovement) {
		t.Errorf("past the top rank: %v", err)
	}
	// 3 000 votes spent, half of them stay in the worth.
	if got := c.Worth(rules, f); got != worth-3_000*100+1_500*100 {
		t.Errorf("worth %d, was %d", got, worth)
	}

	before := f.Capacity(rules, user.GoodGrain)
	if _, err := c.Improve(rules, owner, f.ID(), user.ImproveStorage); err != nil {
		t.Fatal(err)
	}
	if after := f.Capacity(rules, user.GoodGrain); after != before*3/2 {
		t.Errorf("storage held %d, then %d", before, after)
	}

	c.Tick(rules, cityStart.Add(time.Hour))
	if f.Hours()[0].Made <= plain.Hours()[0].Made {
		t.Errorf("machines of rank 3 made %d, plain ones %d", f.Hours()[0].Made, plain.Hours()[0].Made)
	}
}

// unlucky is the luck of a city where everything happens: the first event
// of each list, the first good ordered.
func unlucky(time.Time, uint64, uuid.UUID) float64 {
	return 0
}

func TestEvents(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	c.SetLuck(unlucky)
	owner := uuid.New()
	farm := found(t, c, rules, owner, user.FirmFarm)
	calm := newCity()
	plain := found(t, calm, rules, owner, user.FirmFarm)

	c.Tick(rules, cityStart.Add(time.Hour))
	calm.Tick(rules, cityStart.Add(time.Hour))

	news := c.News()
	kinds := map[user.EventKind]bool{}
	for _, e := range news {
		kinds[e.Kind] = true
	}
	if !kinds[user.EventDrought] || !kinds[user.EventBreakdown] {
		t.Fatalf("news %+v", news)
	}
	// The drought: six tenths of the grain.
	if made, want := farm.Hours()[0].Made, plain.Hours()[0].Made*6/10; made < want-1 || made > want+1 {
		t.Errorf("made %d in the drought, want %d", made, want)
	}
	if farm.Wear() < 25 {
		t.Errorf("a breakdown wore %.1f", farm.Wear())
	}
	if len(calm.News()) != 0 {
		t.Errorf("a calm city has news")
	}
}

func TestStrike(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	c.SetLuck(unlucky)
	owner := uuid.New()
	f := found(t, c, rules, owner, user.FirmFarm)
	if err := c.Staff(rules, owner, f.ID(), 10, rules.Wage/2); err != nil {
		t.Fatal(err)
	}

	c.Tick(rules, cityStart.Add(2*time.Hour))
	if !f.IdleUntil().After(cityStart) {
		t.Fatalf("no strike at half the wage")
	}
	for _, h := range f.Hours() {
		if h.Made != 0 || h.Wages != 0 {
			t.Errorf("an hour of the strike made %d and paid %d", h.Made, h.Wages)
		}
	}
}

func TestContracts(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := user.NewCity(cityStart)
	c.SetLuck(unlucky)
	owner := uuid.New()
	farm := found(t, c, rules, owner, user.FirmFarm)
	mine := found(t, c, rules, owner, user.FirmMine)
	c.Tick(rules, cityStart.Add(time.Hour))
	c.SetLuck(user.CalmLuck)

	contracts := c.Contracts()
	if len(contracts) != 1 || contracts[0].Good != user.GoodGrain || contracts[0].IsTaken() {
		t.Fatalf("contracts %+v", contracts)
	}
	k := contracts[0]
	if k.Price != 25*130/100 || k.Units != 800 {
		t.Errorf("an order of %d grain at %d", k.Units, k.Price)
	}

	now := cityStart.Add(time.Hour)
	if err := c.Take(owner, mine.ID(), k.ID, now); !errors.Is(err, user.ErrFirmGood) {
		t.Errorf("a mine taking grain: %v", err)
	}
	if err := c.Take(owner, farm.ID(), k.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := c.Take(owner, farm.ID(), k.ID, now); !errors.Is(err, user.ErrContractTaken) {
		t.Errorf("taking twice: %v", err)
	}

	if _, err := c.Deliver(owner, k.ID, k.Units+1, now); !errors.Is(err, user.ErrDeliver) {
		t.Errorf("delivering more: %v", err)
	}
	units := farm.Stock(user.GoodGrain)
	if units == 0 {
		if err := c.SetAuto(owner, farm.ID(), user.GoodGrain, user.Auto{}); err != nil {
			t.Fatal(err)
		}
		c.Tick(rules, cityStart.Add(3*time.Hour))
		now = cityStart.Add(3 * time.Hour)
		units = farm.Stock(user.GoodGrain)
	}

	cash := farm.Cash()
	got, err := c.Deliver(owner, k.ID, units, now)
	if err != nil || got != units*k.Price || farm.Cash() != cash+got || farm.Stock(user.GoodGrain) != 0 {
		t.Fatalf("deliver %d: %d, %v", units, got, err)
	}

	// The term ends: the fine is a tenth of what is left.
	cash = farm.Cash()
	left := k.Units - units
	c.Tick(rules, now.Add(25*time.Hour))
	if len(c.Contracts()) != 0 {
		t.Fatalf("the order stays after its term")
	}
	if fine := left * k.Price / 10; farm.Cash() > cash-fine+farm.Profit()+fine {
		t.Errorf("no fine: cash %d, was %d", farm.Cash(), cash)
	}
}
