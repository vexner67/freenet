package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func kiosk(t *testing.T) user.BusinessPlan {
	t.Helper()

	plan, err := user.DefaultBusinessRules().Plan(user.BusinessKiosk)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	return plan
}

func TestNewBusinessRules(t *testing.T) {
	t.Parallel()

	def := user.DefaultBusinessRules()
	if _, err := user.NewBusinessRules(def.Plans(), def.Tax()); err != nil {
		t.Fatalf("NewBusinessRules of the defaults: %v", err)
	}

	tests := []struct {
		name   string
		change func(p []user.BusinessPlan, tax *int) []user.BusinessPlan
		want   error
	}{
		{"a kind missing", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { return p[1:] }, user.ErrBusinessPlans},
		{"out of order", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan {
			p[0], p[1] = p[1], p[0]
			return p
		}, user.ErrBusinessPlans},
		{"free", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[0].Price = 0; return p }, user.ErrBusinessPlanPrice},
		{"no levels", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[1].Levels = 0; return p }, user.ErrBusinessLevels},
		{"no fair price", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[2].Fair = 0; return p }, user.ErrBusinessDemand},
		{"no storage", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[0].Storage = 0; return p }, user.ErrBusinessCapacity},
		{"wear past full", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[0].Wear = 10_001; return p }, user.ErrBusinessCosts},
		{"an ad of no hours", func(p []user.BusinessPlan, _ *int) []user.BusinessPlan { p[3].AdHours = 0; return p }, user.ErrBusinessAd},
		{"tax past 100", func(p []user.BusinessPlan, tax *int) []user.BusinessPlan { *tax = 101; return p }, user.ErrBusinessTax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tax := def.Tax()
			plans := tt.change(def.Plans(), &tax)
			if _, err := user.NewBusinessRules(plans, tax); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBusiness_Earns(t *testing.T) {
	t.Parallel()

	plan := kiosk(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	b, cost, err := user.NewBusiness(plan, now).Restocked(plan, 100)
	if err != nil || cost != 10 {
		t.Fatalf("Restocked = %d, %v, want 10 votes", cost, err)
	}

	// An hour: 10 customers buy at 0.20, one worker costs 0.20 and the
	// rent 0.10.
	b = b.At(plan, now.Add(time.Hour))
	st := b.State()
	if st.Stock != 90 {
		t.Errorf("stock = %d, want 90", st.Stock)
	}
	if st.Cash != 170 {
		t.Errorf("cash = %d, want 170", st.Cash)
	}
	if st.Condition != user.FullCondition-plan.Wear {
		t.Errorf("condition = %d, want %d", st.Condition, user.FullCondition-plan.Wear)
	}

	b, votes, tax, err := b.Collected(5)
	if err != nil || votes != 0 || tax != 1 {
		t.Fatalf("Collected = %d, %d, %v, want 0 and a tax of 1", votes, tax, err)
	}
	if b.Cash() != 70 {
		t.Errorf("cash after = %d, want 70", b.Cash())
	}
	if _, _, _, err = b.Collected(5); !errors.Is(err, user.ErrNothingToCollect) {
		t.Errorf("Collected of less than a vote: %v", err)
	}
}

func TestBusiness_TillAndDebt(t *testing.T) {
	t.Parallel()

	plan := kiosk(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	b, _, err := user.NewBusiness(plan, now).Restocked(plan, plan.Storage)
	if err != nil {
		t.Fatal(err)
	}

	// A day of sales fills the till: what it does not hold is lost.
	b = b.At(plan, now.Add(29*time.Hour))
	if b.Cash() != plan.Till*100 {
		t.Errorf("cash = %d, want the full till %d", b.Cash(), plan.Till*100)
	}

	// Without goods the wages and the rent run into debt until the
	// business goes bankrupt.
	b = b.At(plan, now.Add(60*24*time.Hour))
	if !b.Bankrupt() {
		t.Fatalf("not bankrupt after two months without goods: %+v", b.State())
	}
	if _, _, err = b.Upgraded(plan); !errors.Is(err, user.ErrBusinessBankrupt) {
		t.Errorf("Upgraded of a bankrupt: %v", err)
	}
	if votes, err := b.Resale(); err != nil || votes != 0 {
		t.Errorf("Resale of a bankrupt = %d, %v, want nothing", votes, err)
	}
}

func TestBusiness_Price(t *testing.T) {
	t.Parallel()

	plan := kiosk(t)
	now := time.Now()
	b := user.NewBusiness(plan, now)

	if _, err := b.Managed(plan, 0, 1); !errors.Is(err, user.ErrBusinessSetPrice) {
		t.Errorf("price 0: %v", err)
	}
	if _, err := b.Managed(plan, 20, plan.Staff+1); !errors.Is(err, user.ErrBusinessSetStaff) {
		t.Errorf("staff past the level: %v", err)
	}

	dear, err := b.Managed(plan, 2*plan.Fair, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h := dear.Hour(plan, now); h.Customers != 0 {
		t.Errorf("customers at twice the fair price = %v, want none", h.Customers)
	}

	if h := b.Hour(plan, now); h.Served != 0 || h.Customers != float64(plan.Demand) {
		t.Errorf("hour without goods = %+v, want customers and nobody served", h)
	}
}

func TestBusiness_Actions(t *testing.T) {
	t.Parallel()

	plan := kiosk(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	b := user.NewBusiness(plan, now)

	b, cost, err := b.Upgraded(plan)
	if err != nil || cost != plan.Upgrade || b.Level() != 2 {
		t.Fatalf("Upgraded = %d, %v, level %d", cost, err, b.Level())
	}
	if _, _, err = b.Restocked(plan, plan.StorageAt(2)+1); !errors.Is(err, user.ErrBusinessStock) {
		t.Errorf("restock past the storage: %v", err)
	}

	if _, _, err = b.Repaired(plan); !errors.Is(err, user.ErrBusinessIntact) {
		t.Errorf("repair of full condition: %v", err)
	}
	worn := b.At(plan, now.Add(15*time.Hour))
	if _, cost, err = worn.Repaired(plan); err != nil || cost != 2*plan.Repair {
		t.Errorf("Repaired of 1.5 %% = %d, %v, want two percents", cost, err)
	}

	b, cost, err = b.Advertised(plan, now)
	if err != nil || cost != plan.AdPrice {
		t.Fatalf("Advertised = %d, %v", cost, err)
	}
	b, _, _ = b.Advertised(plan, now)
	if until := b.State().AdUntil; !until.Equal(now.Add(2 * time.Duration(plan.AdHours) * time.Hour)) {
		t.Errorf("two campaigns end %v", until)
	}

	b, err = b.Deposited(3)
	if err != nil || b.Cash() != 300 {
		t.Fatalf("Deposited = %d, %v", b.Cash(), err)
	}
	if votes, err := b.Resale(); err != nil || votes != (plan.Price+plan.Upgrade)/2+3 {
		t.Errorf("Resale = %d, %v", votes, err)
	}

	indebted := user.RestoreBusiness(user.BusinessState{Kind: plan.Kind, Level: 1, Cash: -1})
	if _, err = indebted.Resale(); !errors.Is(err, user.ErrBusinessInDebt) {
		t.Errorf("Resale in debt: %v", err)
	}
}
