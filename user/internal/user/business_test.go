package user_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

var businessNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func plan(t *testing.T, kind user.BusinessKind) user.BusinessPlan {
	t.Helper()

	p, err := user.DefaultBusinessRules().Plan(kind)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	return p
}

func newBusiness(t *testing.T, kind user.BusinessKind) user.Business {
	t.Helper()

	b, err := user.NewBusiness(plan(t, kind), 42, businessNow)
	if err != nil {
		t.Fatalf("NewBusiness: %v", err)
	}

	return b
}

// hire hires a candidate of prof, looking through the windows from now on,
// and the time it was hired at.
func hire(t *testing.T, b user.Business, prof user.Profession, now time.Time) (user.Business, time.Time) {
	t.Helper()

	p, r := plan(t, b.Kind()), user.DefaultBusinessRules()
	for range 200 {
		for _, c := range b.Candidates(r, now) {
			if c.Worker.Profession != prof {
				continue
			}

			hired, err := b.At(p, r, now).Hired(p, r, c.ID, now)
			if err != nil {
				t.Fatalf("Hired: %v", err)
			}

			return hired, now
		}
		now = user.CandidatesRenewAt(now)
	}

	t.Fatalf("no candidate of %s", prof)

	return user.Business{}, now
}

// stocked is b with its storage full of every supply, ordered from the
// express supplier and come.
func stocked(t *testing.T, b user.Business, now time.Time) (user.Business, time.Time) {
	t.Helper()

	r := user.DefaultBusinessRules()
	spec, _ := user.BusinessCatalog(b.Kind())
	for _, s := range spec.Supplies {
		units := int64(math.Floor(float64(b.StorageOf(s)-b.OnTheWay(s.Supply)) - b.Stock(s.Supply)))
		if units < 1 {
			continue
		}

		var err error
		if b, _, err = b.Ordered(r, s.Supply, user.SupplierExpress, units, now); err != nil {
			t.Fatalf("Ordered %s: %v", s.Supply, err)
		}
	}

	now = now.Add(time.Hour)

	return b.At(plan(t, b.Kind()), r, now), now
}

func TestNewBusinessRules(t *testing.T) {
	t.Parallel()

	def := user.DefaultBusinessRules()
	if _, err := user.NewBusinessRules(def.Plans(), def.Tax(), def.Wages(), def.Goods()); err != nil {
		t.Fatalf("NewBusinessRules of the defaults: %v", err)
	}

	tests := []struct {
		name   string
		change func(p []user.BusinessPlan, n []int) []user.BusinessPlan
		want   error
	}{
		{"a kind missing", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { return p[1:] }, user.ErrBusinessPlans},
		{"out of order", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan {
			p[0], p[1] = p[1], p[0]
			return p
		}, user.ErrBusinessPlans},
		{"free", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[0].Price = 0; return p }, user.ErrBusinessPlanPrice},
		{"no levels", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[1].Levels = 0; return p }, user.ErrBusinessLevels},
		{"demand below zero", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[2].Demand = -1; return p }, user.ErrBusinessPercent},
		{"no seats", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[0].Staff = 0; return p }, user.ErrBusinessCapacity},
		{"wear past full", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[0].Wear = 10_001; return p }, user.ErrBusinessCosts},
		{"an ad of no hours", func(p []user.BusinessPlan, _ []int) []user.BusinessPlan { p[3].AdHours = 0; return p }, user.ErrBusinessAd},
		{"tax past 100", func(p []user.BusinessPlan, n []int) []user.BusinessPlan { n[0] = 101; return p }, user.ErrBusinessTax},
		{"wages below zero", func(p []user.BusinessPlan, n []int) []user.BusinessPlan { n[1] = -1; return p }, user.ErrBusinessPercent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := []int{def.Tax(), def.Wages(), def.Goods()}
			plans := tt.change(def.Plans(), n)
			if _, err := user.NewBusinessRules(plans, n[0], n[1], n[2]); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBusinessCatalog(t *testing.T) {
	t.Parallel()

	for _, kind := range user.BusinessKinds {
		spec, err := user.BusinessCatalog(kind)
		if err != nil {
			t.Fatalf("BusinessCatalog(%s): %v", kind, err)
		}
		if len(spec.Serving()) == 0 {
			t.Errorf("%s serves nobody", kind)
		}
		for _, o := range spec.Offers {
			if _, err := spec.Profession(o.Profession); err != nil {
				t.Errorf("%s: %s served by unknown %s", kind, o.Offer, o.Profession)
			}
			for _, in := range o.Recipe {
				if _, err := spec.Supply(in.Supply); err != nil {
					t.Errorf("%s: %s uses unknown %s", kind, o.Offer, in.Supply)
				}
			}
		}
	}
}

func TestBusiness_NothingWithoutWorkersAndSupplies(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk).At(p, r, businessNow.Add(5*time.Hour))
	for _, h := range b.Hours() {
		if h.Income != 0 {
			t.Fatalf("income %v without workers", h.Income)
		}
	}
	if b.Cash() >= 0 {
		t.Fatalf("cash = %d, want the rent to make a debt", b.Cash())
	}

	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)
	b = b.At(p, r, now.Add(5*time.Hour))
	for _, h := range b.Hours() {
		if h.Income != 0 {
			t.Fatalf("income %v without supplies", h.Income)
		}
	}
}

func TestBusiness_Sells(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)
	b, now = stocked(t, b, now)
	before := b.Stock(user.SupplyDrinks)

	b = b.At(p, r, now.Add(3*time.Hour))
	var income, sold float64
	for _, h := range b.Hours() {
		income += h.Income
		sold += h.Sold[user.OfferDrinks]
	}
	if income <= 0 || sold <= 0 {
		t.Fatalf("income %v, sold %v, want sales", income, sold)
	}
	if got := before - b.Stock(user.SupplyDrinks); got < sold-1e-6 || got > sold+1e-6 {
		t.Fatalf("drinks used %v, sold %v", got, sold)
	}

	rates := b.Rates(p, r, now)
	if rates.Load[user.ProfSeller] <= 1 {
		t.Fatalf("one seller has load %v, want more than he can serve", rates.Load[user.ProfSeller])
	}
}

func TestBusiness_DearerSellsLess(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)
	fair := b.Rates(p, r, businessNow).Offers[0].Demand

	dear, err := b.Priced(user.OfferDrinks, b.Price(user.OfferDrinks)*2)
	if err != nil {
		t.Fatalf("Priced: %v", err)
	}
	if got := dear.Rates(p, r, businessNow).Offers[0].Demand; got != 0 || fair <= 0 {
		t.Fatalf("demand %v at twice the fair price (%v at fair), want 0", got, fair)
	}

	if _, err := b.Priced(user.OfferDrinks, 0); !errors.Is(err, user.ErrBusinessSetPrice) {
		t.Fatalf("price 0: %v", err)
	}
	if _, err := b.Priced(user.OfferCoffee, 10); !errors.Is(err, user.ErrUnknownOffer) {
		t.Fatalf("coffee of a kiosk: %v", err)
	}
}

func TestBusiness_Orders(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)

	if _, _, err := b.Ordered(r, user.SupplyDrinks, user.SupplierReliable, 201, businessNow); !errors.Is(err, user.ErrBusinessOrder) {
		t.Fatalf("past the storage: %v", err)
	}
	if _, _, err := b.Ordered(r, user.SupplyBeans, user.SupplierReliable, 1, businessNow); !errors.Is(err, user.ErrUnknownSupply) {
		t.Fatalf("beans of a kiosk: %v", err)
	}

	b, cost, err := b.Ordered(r, user.SupplyDrinks, user.SupplierReliable, 100, businessNow)
	if err != nil || cost != 4 {
		t.Fatalf("Ordered = %d, %v; want 4 votes", cost, err)
	}
	if got := b.OnTheWay(user.SupplyDrinks); got != 100 {
		t.Fatalf("on the way %d, want 100", got)
	}

	b = b.At(p, r, businessNow.Add(time.Hour))
	if b.Stock(user.SupplyDrinks) != 0 {
		t.Fatalf("the reliable supplier came in an hour")
	}

	b = b.At(p, r, businessNow.Add(2*time.Hour))
	if got := b.Stock(user.SupplyDrinks); got != 90 {
		t.Fatalf("stock %v, want 90: a tenth lost without a storekeeper", got)
	}
}

func TestBusiness_StorekeeperKeepsDeliveries(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfStorekeeper, businessNow)
	b, _, err := b.Ordered(r, user.SupplyDrinks, user.SupplierReliable, 100, now)
	if err != nil {
		t.Fatalf("Ordered: %v", err)
	}

	b = b.At(p, r, now.Add(2*time.Hour))
	if got := b.Stock(user.SupplyDrinks); got != 100 {
		t.Fatalf("stock %v, want 100 with a storekeeper", got)
	}
}

func TestBusiness_Contracts(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)
	if _, err := b.Contracted(user.SupplyDrinks, user.SupplierReliable, 300, 10); !errors.Is(err, user.ErrBusinessContract) {
		t.Fatalf("below past the storage: %v", err)
	}

	b, err := b.Contracted(user.SupplyDrinks, user.SupplierReliable, 50, 100)
	if err != nil {
		t.Fatalf("Contracted: %v", err)
	}
	b, err = b.Deposited(20)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}

	if got := b.At(p, r, businessNow.Add(time.Hour)).OnTheWay(user.SupplyDrinks); got != 0 {
		t.Fatalf("a contract ran without a buyer: %d on the way", got)
	}

	b, now := hire(t, b, user.ProfBuyer, businessNow)
	b = b.At(p, r, now.Add(time.Hour))
	if b.OnTheWay(user.SupplyDrinks)+int64(b.Stock(user.SupplyDrinks)) == 0 {
		t.Fatalf("the buyer ordered nothing")
	}

	if b, err = b.Contracted(user.SupplyDrinks, user.SupplierReliable, 0, 0); err != nil || len(b.Contracts()) != 0 {
		t.Fatalf("ending the contract: %v, %d left", err, len(b.Contracts()))
	}
}

func TestBusiness_Spoils(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfStorekeeper, businessNow)
	b, _, err := b.Ordered(r, user.SupplyPapers, user.SupplierExpress, 80, now)
	if err != nil {
		t.Fatalf("Ordered: %v", err)
	}

	b = b.At(p, r, now.Add(time.Hour))
	full := b.Stock(user.SupplyPapers)
	b = b.At(p, r, now.Add(13*time.Hour))
	if got := b.Stock(user.SupplyPapers); got >= full || got <= 0 {
		t.Fatalf("papers %v after 12 hours of %v, want some spoiled", got, full)
	}
}

func TestBusiness_Staff(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)

	candidates := b.Candidates(r, businessNow)
	if len(candidates) != user.CandidatesShown {
		t.Fatalf("%d candidates, want %d", len(candidates), user.CandidatesShown)
	}
	for _, c := range candidates[:2] {
		if c.Worker.Profession != user.ProfSeller {
			t.Fatalf("the first candidates are %s, want sellers", c.Worker.Profession)
		}
	}

	b, err := b.Hired(p, r, candidates[0].ID, businessNow)
	if err != nil {
		t.Fatalf("Hired: %v", err)
	}
	if _, err := b.Hired(p, r, candidates[0].ID, businessNow); !errors.Is(err, user.ErrCandidateGone) {
		t.Fatalf("hiring twice: %v", err)
	}
	if got := len(b.Candidates(r, businessNow)); got != user.CandidatesShown-1 {
		t.Fatalf("%d candidates after hiring, want %d", got, user.CandidatesShown-1)
	}
	if _, err := b.Hired(p, r, candidates[1].ID, user.CandidatesRenewAt(businessNow)); !errors.Is(err, user.ErrCandidateGone) {
		t.Fatalf("hiring an old candidate: %v", err)
	}

	for _, c := range candidates[1:p.Staff] {
		if b, err = b.Hired(p, r, c.ID, businessNow); err != nil {
			t.Fatalf("Hired: %v", err)
		}
	}
	if len(b.Workers()) != p.Staff {
		t.Fatalf("%d workers, want %d", len(b.Workers()), p.Staff)
	}

	id := b.Workers()[0].ID
	wage := b.Workers()[0].Wage
	if b, err = b.Raised(id); err != nil || b.Workers()[0].Wage <= wage {
		t.Fatalf("Raised: %v, wage %d from %d", err, b.Workers()[0].Wage, wage)
	}
	if _, cost, err := b.Bonused(id); err != nil || cost < 1 {
		t.Fatalf("Bonused = %d, %v", cost, err)
	}
	if b, err = b.Fired(id, businessNow); err != nil || len(b.Workers()) != p.Staff-1 {
		t.Fatalf("Fired: %v", err)
	}
	if _, err := b.Fired(id, businessNow); !errors.Is(err, user.ErrWorkerNotFound) {
		t.Fatalf("firing twice: %v", err)
	}
}

func TestBusiness_NoSeats(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	p.Staff = 1
	b := newBusiness(t, user.BusinessKiosk)
	candidates := b.Candidates(r, businessNow)

	b, err := b.Hired(p, r, candidates[0].ID, businessNow)
	if err != nil {
		t.Fatalf("Hired: %v", err)
	}
	if _, err := b.Hired(p, r, candidates[1].ID, businessNow); !errors.Is(err, user.ErrNoSeats) {
		t.Fatalf("past the seats: %v", err)
	}
}

func TestBusiness_UnpaidWorkersQuit(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	p.Debt = 1_000
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)

	b = b.At(p, r, now.Add(80*time.Hour))
	if len(b.Workers()) != 0 {
		t.Fatalf("a worker unpaid for 80 hours stays, mood %v", b.Workers()[0].Mood)
	}

	quit := false
	for _, n := range b.Notes() {
		quit = quit || n.Kind == user.NoteQuit
	}
	if !quit {
		t.Fatalf("no note of quitting")
	}
}

func TestBusiness_Learns(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	p.Rent, p.Demand, p.Till = 0, 0, 100_000
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)
	b, err := b.Deposited(1_000)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}

	w := b.Workers()[0]
	if w.Skill == user.MaxSkill {
		t.Skip("the candidate has the top skill")
	}

	b = b.At(p, r, now.Add(time.Duration(w.NextSkillHours()+1)*time.Hour))
	if got := b.Workers()[0].Skill; got != w.Skill+1 {
		t.Fatalf("skill %d after %v hours, want %d", got, w.NextSkillHours(), w.Skill+1)
	}
}

func TestBusiness_TillAndDebt(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)

	b, err := b.Deposited(1_000)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}
	if b = b.At(p, r, businessNow.Add(time.Hour)); float64(b.Cash()) > b.TillAt(p) {
		t.Fatalf("cash %d past the till %v", b.Cash(), b.TillAt(p))
	}

	b = newBusiness(t, user.BusinessKiosk).At(p, r, businessNow.Add(2_000*time.Hour))
	if !b.Bankrupt() {
		t.Fatalf("a kiosk paying rent for 2000 hours with nothing is not bankrupt: cash %d", b.Cash())
	}
	if _, err := b.Deposited(1); !errors.Is(err, user.ErrBusinessBankrupt) {
		t.Fatalf("depositing into a bankrupt business: %v", err)
	}
	if resale, err := b.Resale(); err != nil || resale != 0 {
		t.Fatalf("Resale of a bankrupt = %d, %v", resale, err)
	}

	debt := newBusiness(t, user.BusinessKiosk).At(p, r, businessNow.Add(5*time.Hour))
	if _, err := debt.Resale(); !errors.Is(err, user.ErrBusinessInDebt) {
		t.Fatalf("Resale in debt: %v", err)
	}
}

func TestBusiness_Collect(t *testing.T) {
	t.Parallel()

	r := user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)
	if _, _, _, err := b.Collected(r); !errors.Is(err, user.ErrNothingToCollect) {
		t.Fatalf("collecting nothing: %v", err)
	}

	b, err := b.Deposited(20)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}

	after, got, tax, err := b.Collected(r)
	if err != nil || got != 19 || tax != 1 || after.Cash() != 0 {
		t.Fatalf("Collected = %d, %d, %v; cash %d", got, tax, err, after.Cash())
	}

	if resale, err := b.Resale(); err != nil || resale != 250+20 {
		t.Fatalf("Resale = %d, %v; want 270", resale, err)
	}
}

func TestBusiness_UpgradeAndRepair(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b := newBusiness(t, user.BusinessKiosk)
	if _, _, err := b.Repaired(p, businessNow); !errors.Is(err, user.ErrBusinessIntact) {
		t.Fatalf("repairing new premises: %v", err)
	}

	up, cost, err := b.Upgraded(p)
	if err != nil || cost != p.Upgrade || up.Level() != 2 || up.Seats(p) != 2*p.Staff {
		t.Fatalf("Upgraded = %d, %v; level %d", cost, err, up.Level())
	}

	worn := b.At(p, r, businessNow.Add(100*time.Hour))
	if worn.Condition() >= user.FullCondition {
		t.Fatalf("no wear in 100 hours")
	}

	fixed, cost, err := worn.Repaired(p, businessNow.Add(100*time.Hour))
	if err != nil || cost < 1 || fixed.Condition() != user.FullCondition {
		t.Fatalf("Repaired = %d, %v; condition %d", cost, err, fixed.Condition())
	}

	top := b
	for top.Level() < p.Levels {
		top, _, _ = top.Upgraded(p)
	}
	if _, _, err := top.Upgraded(p); !errors.Is(err, user.ErrBusinessTopLevel) {
		t.Fatalf("past the top: %v", err)
	}
}

func TestBusiness_Events(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessCafe), user.DefaultBusinessRules()
	p.Rent, p.Wear = 0, 0
	b, err := newBusiness(t, user.BusinessCafe).Deposited(10_000)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}

	spec, _ := user.BusinessCatalog(user.BusinessCafe)
	if _, err := b.Decided(spec, user.OptionFine, businessNow); !errors.Is(err, user.ErrNoEvent) {
		t.Fatalf("deciding nothing: %v", err)
	}

	now := businessNow
	seen := map[user.EventKind]bool{}
	for range 400 {
		now = now.Add(time.Hour)
		b = b.At(p, r, now)

		e, ok := b.Event()
		if !ok {
			continue
		}

		seen[e.Kind] = true
		if _, err := b.Decided(spec, "nonsense", now); !errors.Is(err, user.ErrUnknownOption) {
			t.Fatalf("an unknown answer: %v", err)
		}

		decided, err := b.Decided(spec, e.Options()[0], now)
		if err != nil {
			t.Fatalf("Decided %s: %v", e.Kind, err)
		}
		if _, ok := decided.Event(); ok {
			t.Fatalf("the event stays after the answer")
		}
		b = decided
	}

	if len(seen) == 0 {
		t.Fatalf("nothing happened in 400 hours")
	}
}

func TestBusiness_EventsExpire(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	p.Rent, p.Wear = 0, 0
	b, err := newBusiness(t, user.BusinessKiosk).Deposited(1_000)
	if err != nil {
		t.Fatalf("Deposited: %v", err)
	}

	now := businessNow
	for range 400 {
		now = now.Add(time.Hour)
		b = b.At(p, r, now)

		if e, ok := b.Event(); ok {
			if later := b.At(p, r, e.Until.Add(time.Minute)); later.Notes() == nil {
				t.Fatalf("no notes")
			}
			if later, _ := b.At(p, r, e.Until.Add(time.Minute)).Event(); later.Until.Equal(e.Until) {
				t.Fatalf("the event of %v waits past its day", e.Until)
			}

			return
		}
	}

	t.Fatalf("nothing happened in 400 hours")
}

func TestBusiness_StateRoundTrip(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)
	b, now = stocked(t, b, now)
	b = b.At(p, r, now.Add(3*time.Hour))

	restored := user.RestoreBusiness(b.State())
	if restored.Cash() != b.Cash() || len(restored.Workers()) != 1 || restored.Stock(user.SupplyDrinks) != b.Stock(user.SupplyDrinks) ||
		len(restored.Hours()) != len(b.Hours()) || len(restored.Notes()) != len(b.Notes()) {
		t.Fatalf("restored %+v, want %+v", restored.State(), b.State())
	}

	later := now.Add(5 * time.Hour)
	if got, want := restored.At(p, r, later).Cash(), b.At(p, r, later).Cash(); got != want {
		t.Fatalf("the restored business runs to %d, the kept one to %d", got, want)
	}
}

func TestBusiness_DoesNotChangeTheOriginal(t *testing.T) {
	t.Parallel()

	p, r := plan(t, user.BusinessKiosk), user.DefaultBusinessRules()
	b, now := hire(t, newBusiness(t, user.BusinessKiosk), user.ProfSeller, businessNow)
	b, now = stocked(t, b, now)

	stock := b.Stock(user.SupplyDrinks)
	_ = b.At(p, r, now.Add(5*time.Hour))
	if b.Stock(user.SupplyDrinks) != stock {
		t.Fatalf("At changed the original stock")
	}
}

func TestBusiness_PaysBackWellRun(t *testing.T) {
	t.Parallel()

	r := user.DefaultBusinessRules()
	for _, kind := range user.BusinessKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			p := plan(t, kind)
			spec, _ := user.BusinessCatalog(kind)
			b, now := newBusiness(t, kind), businessNow
			for _, prof := range append(spec.Serving(), spec.Serving()...) {
				if len(b.Workers()) < b.Seats(p)-1 {
					b, now = hire(t, b, prof, now)
				}
			}
			b, now = hire(t, b, user.ProfStorekeeper, now)

			var err error
			if b, err = b.Deposited(p.Till); err != nil {
				t.Fatalf("Deposited: %v", err)
			}

			var earned int64
			for range 7 * 24 {
				b, now = stocked(t, b, now)
				if b.Cash() >= 100 {
					var got int64
					b, got, _, _ = b.Collected(r)
					earned += got
				}
			}

			if earned <= 0 {
				t.Fatalf("a %s run well earned %d in a week", kind, earned)
			}
		})
	}
}
