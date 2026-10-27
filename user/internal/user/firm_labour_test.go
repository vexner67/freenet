package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

// hired and left are what the reports of the enterprise count.
func hiredAndLeft(f *user.Firm) (hired, left int64) {
	for _, h := range f.Hours() {
		hired += h.Hired
		left += h.Left
	}

	return hired, left
}

func TestFoundingBringsPeople(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	f := found(t, c, rules, uuid.New(), user.FirmFarm)

	l := c.Labour(rules)
	if l.People != 10 || l.Employed != 10 || l.Jobs != 10 || l.Wage != rules.Wage || f.Workers() != 10 {
		t.Fatalf("a farm founded: %+v, %d workers", l, f.Workers())
	}
	if hired, _ := hiredAndLeft(f); hired != 10 {
		t.Errorf("hired %d", hired)
	}
}

// TestVacanciesFill: people come to the city for its jobs, never all of
// them.
func TestVacanciesFill(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner := uuid.New()
	f := found(t, c, rules, owner, user.FirmFarm)
	if err := c.Deposit(owner, f.ID(), 10_000); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Upgrade(rules, owner, f.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Staff(rules, owner, f.ID(), 20, rules.Wage, cityStart); err != nil {
		t.Fatal(err)
	}

	c.Tick(rules, cityStart.Add(time.Hour))
	if f.Workers() != 11 {
		t.Errorf("an hour after: %d workers", f.Workers())
	}

	c.Tick(rules, cityStart.Add(24*time.Hour))
	if f.Workers() != 19 || c.Labour(rules).People != 19 {
		t.Errorf("a day after: %d workers, %+v", f.Workers(), c.Labour(rules))
	}
}

// TestBetterPayDraws: workers leave the enterprise paying below the
// average for one paying more with vacancies.
func TestBetterPayDraws(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	poor, rich := uuid.New(), uuid.New()
	low := found(t, c, rules, poor, user.FirmFarm)
	high := found(t, c, rules, rich, user.FirmMine)
	if err := c.Deposit(rich, high.ID(), 10_000); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Upgrade(rules, rich, high.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Staff(rules, rich, high.ID(), 20, rules.Wage*2, cityStart); err != nil {
		t.Fatal(err)
	}

	c.Tick(rules, cityStart.Add(time.Hour))
	if low.Workers() >= 10 || high.Workers() <= 10 || low.Workers()+high.Workers() != c.Labour(rules).Employed {
		t.Fatalf("the farm keeps %d, the mine has %d", low.Workers(), high.Workers())
	}
	if _, left := hiredAndLeft(low); left != int64(10-low.Workers()) {
		t.Errorf("the farm reports %d left", left)
	}

	// Paying as much, the farm keeps its workers.
	if err := c.Staff(rules, poor, low.ID(), 10, rules.Wage*2, cityStart.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	kept := low.Workers()
	c.Tick(rules, cityStart.Add(2*time.Hour))
	if low.Workers() < kept {
		t.Errorf("paying the same, the farm lost workers: %d of %d", low.Workers(), kept)
	}
}

// TestClosedEnterpriseFrees: the workers of an enterprise closed are out
// of work, and those not hired leave the city in time.
func TestClosedEnterpriseFrees(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner := uuid.New()
	farm := found(t, c, rules, owner, user.FirmFarm)
	found(t, c, rules, owner, user.FirmMine)

	if _, err := c.Liquidate(rules, owner, farm.ID()); err != nil {
		t.Fatal(err)
	}
	if l := c.Labour(rules); l.People != 20 || l.Unemployed() != 10 {
		t.Fatalf("after closing: %+v", l)
	}

	c.Tick(rules, cityStart.Add(48*time.Hour))
	if l := c.Labour(rules); l.People != 10 || l.Employed != 10 {
		t.Errorf("two days after: %+v", l)
	}
}

// TestFoundingHiresTheOutOfWork: a new enterprise hires those out of work
// before people come.
func TestFoundingHiresTheOutOfWork(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner := uuid.New()
	farm := found(t, c, rules, owner, user.FirmFarm)
	if err := c.Staff(rules, owner, farm.ID(), 4, rules.Wage, cityStart); err != nil {
		t.Fatal(err)
	}

	found(t, c, rules, owner, user.FirmMine)
	if l := c.Labour(rules); l.People != 14 || l.Employed != 14 {
		t.Errorf("a mine founded with 6 out of work: %+v", l)
	}
}

// TestCityBeforeLabour: a city kept before the labour market has the
// people it employs, and its enterprises seek the workers they have.
func TestCityBeforeLabour(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	found(t, c, rules, uuid.New(), user.FirmFarm)

	s := c.State()
	s.People = 0
	s.Firms[0].Wanted = 0
	restored := user.RestoreCity(s)
	if l := restored.Labour(rules); l.People != 10 || l.Jobs != 10 || l.Unemployed() != 0 {
		t.Errorf("restored: %+v", l)
	}
}

// TestCityOrdersWhatItMakes: the city orders only the goods its
// enterprises make, and nothing of a city with none.
func TestCityOrdersWhatItMakes(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	empty := user.NewCity(cityStart)
	empty.Tick(rules, cityStart.Add(24*time.Hour))
	if n := len(empty.Contracts()); n != 0 {
		t.Errorf("a city of no enterprises ordered %d", n)
	}

	c := user.NewCity(cityStart)
	owner := uuid.New()
	found(t, c, rules, owner, user.FirmMine)
	found(t, c, rules, owner, user.FirmShop)
	c.Tick(rules, cityStart.Add(24*time.Hour))

	contracts := c.Contracts()
	if len(contracts) == 0 || len(contracts) > 8 {
		t.Fatalf("a day of a mine: %d orders", len(contracts))
	}
	for _, k := range contracts {
		if k.Good != user.GoodOre {
			t.Errorf("the city of a mine and a shop ordered %s", k.Good)
		}
	}
}
