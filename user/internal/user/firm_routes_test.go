package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestSupplyRoutes(t *testing.T) {
	t.Parallel()

	rules := user.DefaultFirmRules()
	c := newCity()
	owner, other := uuid.New(), uuid.New()
	farm := found(t, c, rules, owner, user.FirmFarm)
	mill := found(t, c, rules, owner, user.FirmMill)
	stranger := found(t, c, rules, other, user.FirmMill)

	for _, tc := range []struct {
		name  string
		user  uuid.UUID
		from  uuid.UUID
		route user.SupplyRoute
		want  error
	}{
		{"not the director", other, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, Units: 5}, user.ErrNotDirector},
		{"of another director", owner, farm.ID(), user.SupplyRoute{To: stranger.ID(), Good: user.GoodGrain, Units: 5}, user.ErrRoute},
		{"to itself", owner, farm.ID(), user.SupplyRoute{To: farm.ID(), Good: user.GoodGrain, Units: 5}, user.ErrRoute},
		{"to nobody", owner, farm.ID(), user.SupplyRoute{To: uuid.New(), Good: user.GoodGrain, Units: 5}, user.ErrRoute},
		{"a good not kept", owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodOre, Units: 5}, user.ErrRoute},
		{"a good not used", owner, mill.ID(), user.SupplyRoute{To: farm.ID(), Good: user.GoodGrain, Units: 5}, user.ErrRoute},
		{"no units", owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain}, user.ErrRoute},
		{"kept below zero", owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, Units: 5, From: -1}, user.ErrRoute},
	} {
		if _, err := c.AddRoute(tc.user, tc.from, tc.route); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	// The mill buys no grain by itself: what it gets comes by the supply.
	if err := c.SetAuto(owner, mill.ID(), user.GoodGrain, user.Auto{}); err != nil {
		t.Fatal(err)
	}
	route, err := c.AddRoute(owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, From: 1_000_000, Units: 7})
	if err != nil || len(farm.Routes()) != 1 {
		t.Fatalf("add: %+v, %v", route, err)
	}

	// Below what it must keep, the farm hands nothing.
	c.Tick(rules, cityStart.Add(time.Hour))
	if h := mill.Hours(); len(h) > 0 && h[len(h)-1].Bought != 0 {
		t.Errorf("the mill got %d below the threshold", h[len(h)-1].Bought)
	}

	if err = c.RemoveRoute(owner, farm.ID(), route.ID); err != nil || len(farm.Routes()) != 0 {
		t.Fatalf("remove: %v", err)
	}
	if err = c.RemoveRoute(owner, farm.ID(), route.ID); !errors.Is(err, user.ErrRouteNotFound) {
		t.Errorf("removing again: %v", err)
	}

	if _, err = c.AddRoute(owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, Units: 7}); err != nil {
		t.Fatal(err)
	}
	farmCash, millCash := farm.Cash(), mill.Cash()
	c.Tick(rules, cityStart.Add(2*time.Hour))
	h := mill.Hours()
	if len(h) == 0 || h[len(h)-1].Bought != 7 || h[len(h)-1].Purchases <= 0 {
		t.Fatalf("the mill got %+v", h)
	}
	if fh := farm.Hours(); fh[len(fh)-1].Income < h[len(h)-1].Purchases || farm.Cash() <= farmCash-fh[len(fh)-1].Wages-fh[len(fh)-1].Upkeep || mill.Cash() >= millCash {
		t.Errorf("farm cash %d → %d, mill cash %d → %d", farmCash, farm.Cash(), millCash, mill.Cash())
	}

	// Kept and restored, the supply stays.
	if restored := user.RestoreCity(c.State()); len(mustFirm(t, restored, farm.ID()).Routes()) != 1 {
		t.Error("the supply was lost keeping the city")
	}

	// An enterprise keeps at most MaxRoutes supplies.
	for range user.MaxRoutes - 1 {
		if _, err = c.AddRoute(owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, Units: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = c.AddRoute(owner, farm.ID(), user.SupplyRoute{To: mill.ID(), Good: user.GoodGrain, Units: 1}); !errors.Is(err, user.ErrTooManyRoutes) {
		t.Errorf("one supply too many: %v", err)
	}
}

func mustFirm(t *testing.T, c *user.City, id uuid.UUID) *user.Firm {
	t.Helper()

	f, err := c.Firm(id)
	if err != nil {
		t.Fatal(err)
	}

	return f
}
