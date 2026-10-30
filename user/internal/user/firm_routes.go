package user

import (
	"errors"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Supplies between the enterprises of one director: an enterprise hands a
// good of its storage to another one of the same director every hour by
// itself — so many units once it keeps at least so many, as far as the
// storage of the other holds and its cash pays. The other pays the price
// of a supply between enterprises (halfway between what the state pays
// and asks), with no fee of the exchange: the shareholders of each lose
// nothing to those of the other. A supply stops while the two have
// different directors and goes with either enterprise.
//
// An automatic supply needs no numbers: every hour it hands what the other
// enterprise will use in the next SupplyHours and has not got, out of what
// is left after the orders of the city, sharing that between the
// enterprises that wait for the good by how much each lacks.

// MaxRoutes are the most supplies an enterprise keeps.
const MaxRoutes = 5

// SupplyHours are the hours of work an automatic supply keeps in stock at
// the enterprise it goes to: enough to last through a poor hour.
const SupplyHours = 3

var (
	ErrRoute         = errors.New("a supply goes to another enterprise you direct that works with the good, of 1 unit or more an hour, once from 0 units are kept")
	ErrTooManyRoutes = errors.New("an enterprise keeps at most 5 supplies")
	ErrRouteNotFound = errors.New("there is no such supply")
	ErrNoAutoRoutes  = errors.New("no enterprise of yours needs what another one makes")
)

// SupplyRoute is a supply of an enterprise: Units of Good an hour to the
// enterprise To, once its storage keeps From units or more; an Auto one
// counts what to send by itself.
type SupplyRoute struct {
	ID    uuid.UUID
	To    uuid.UUID
	Good  Good
	From  int64
	Units int64
	Auto  bool
}

// Routes are the supplies of the enterprise.
func (f *Firm) Routes() []SupplyRoute { return slices.Clone(f.routes) }

// AddRoute sets up a supply of the enterprise firmID userID directs to
// another one they direct. ErrNotDirector, ErrFirmNotFound, ErrRoute,
// ErrTooManyRoutes.
func (c *City) AddRoute(userID, firmID uuid.UUID, r SupplyRoute) (SupplyRoute, error) {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return SupplyRoute{}, err
	}

	to, err := c.Firm(r.To)
	if errors.Is(err, ErrFirmNotFound) {
		return SupplyRoute{}, ErrRoute
	}
	if err != nil {
		return SupplyRoute{}, err
	}
	if !routable(f, to, r) || to.directorID != userID {
		return SupplyRoute{}, ErrRoute
	}
	if len(f.routes) >= MaxRoutes {
		return SupplyRoute{}, ErrTooManyRoutes
	}

	if r.ID, err = uuid.NewV7(); err != nil {
		return SupplyRoute{}, err
	}
	f.routes = append(f.routes, r)

	return r, nil
}

// AutoRoutes replaces the supplies of every enterprise userID directs by
// automatic ones, from each to the enterprises of theirs that work with
// what it makes, and says how many there are now; ErrNoAutoRoutes.
func (c *City) AutoRoutes(userID uuid.UUID) (int, error) {
	made := 0
	plans := make(map[*Firm][]SupplyRoute)

	for _, f := range c.firms {
		if f.directorID != userID || f.spec().Output == "" {
			continue
		}

		good := f.spec().Output
		for _, to := range c.firms {
			if to.id == f.id || to.directorID != userID || !to.spec().IsInput(good) || len(plans[f]) >= MaxRoutes {
				continue
			}

			id, err := uuid.NewV7()
			if err != nil {
				return 0, err
			}
			plans[f] = append(plans[f], SupplyRoute{ID: id, To: to.id, Good: good, Auto: true})
			made++
		}
	}

	if made == 0 {
		return 0, ErrNoAutoRoutes
	}

	for _, f := range c.firms {
		if f.directorID == userID {
			f.routes = plans[f]
		}
	}

	return made, nil
}

// lacks is how many units of g the enterprise has to get to last
// SupplyHours at the pace of its workers.
func (c *City) lacks(rules FirmRules, f *Firm, g Good) int64 {
	pace := float64(f.workers) * rules.Productivity(f.wage) * f.condition() * f.machines() * f.spec().perHour(g)

	return max(int64(math.Ceil(pace*SupplyHours))-f.stock[g]-c.Incoming(f.id, g), 0)
}

// kept is what of g the enterprise holds back for the orders of the city:
// all it still owes and a part of the largest order nobody took, so that
// there is something to take it with.
func (c *City) kept(f *Firm, g Good) int64 {
	var owed, open int64
	for _, k := range c.contracts {
		switch {
		case k.Good != g:
		case k.FirmID == f.id:
			owed += k.Units - k.Delivered
		case !k.IsTaken():
			open = max(open, k.Units)
		}
	}

	return min(owed+min(open, f.stock[g]/autoKeepShare), f.stock[g])
}

// autoKeepShare: at most this part of the stock waits for an order of the
// city nobody took, and the rest keeps the enterprises going.
const autoKeepShare = 5

// autoUnits are the units each automatic supply of f hands this hour.
func (c *City) autoUnits(rules FirmRules, f *Firm) map[uuid.UUID]int64 {
	lacking := make(map[uuid.UUID]int64)
	total := make(map[Good]int64)

	for _, r := range f.routes {
		if to, err := c.Firm(r.To); r.Auto && err == nil && to.directorID == f.directorID {
			lacking[r.ID] = c.lacks(rules, to, r.Good)
			total[r.Good] += lacking[r.ID]
		}
	}

	units := make(map[uuid.UUID]int64, len(lacking))
	for _, r := range f.routes {
		if total[r.Good] == 0 {
			continue
		}

		free := f.stock[r.Good] - c.kept(f, r.Good)
		units[r.ID] = min(lacking[r.ID], ceilShare(free, lacking[r.ID], total[r.Good]))
	}

	return units
}

// ceilShare is free*part/whole rounded up, so that nobody who waits gets
// nothing of a small stock.
func ceilShare(free, part, whole int64) int64 {
	share := new(big.Int).Mul(big.NewInt(free), big.NewInt(part))
	share.Add(share, big.NewInt(whole-1))

	return share.Quo(share, big.NewInt(whole)).Int64()
}

// routable is whether a supply of r may go from f to to: another
// enterprise, of a good f keeps and to works with, of sane amounts.
func routable(f, to *Firm, r SupplyRoute) bool {
	return f.id != to.id && slices.Contains(f.spec().Uses(), r.Good) && to.spec().IsInput(r.Good) &&
		r.Units >= 1 && r.Units <= maxFirmUnits && r.From >= 0 && r.From <= maxFirmUnits
}

// RemoveRoute stops a supply of the enterprise firmID userID directs.
// ErrNotDirector, ErrFirmNotFound, ErrRouteNotFound.
func (c *City) RemoveRoute(userID, firmID, routeID uuid.UUID) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	i := slices.IndexFunc(f.routes, func(r SupplyRoute) bool { return r.ID == routeID })
	if i < 0 {
		return ErrRouteNotFound
	}
	f.routes = slices.Delete(f.routes, i, i+1)

	return nil
}

// route hands the supplies of the enterprise in the hour at; a supply to
// an enterprise no longer there goes.
func (c *City) route(rules FirmRules, f *Firm, at time.Time) {
	f.routes = slices.DeleteFunc(f.routes, func(r SupplyRoute) bool {
		_, err := c.Firm(r.To)
		return err != nil
	})

	auto := c.autoUnits(rules, f)

	for _, r := range f.routes {
		to, _ := c.Firm(r.To) // kept above
		if to.directorID != f.directorID || f.stock[r.Good] < r.From {
			continue
		}

		if r.Auto {
			r.Units = auto[r.ID]
		}

		price := c.supplyPrice(rules, r.Good, at)
		if price == 0 {
			continue
		}

		units := min(r.Units, f.stock[r.Good], c.room(rules, to, r.Good), to.cash/price)
		if units <= 0 {
			continue
		}

		value := units * price
		f.stock[r.Good] -= units
		to.stock[r.Good] += units
		to.cash -= value
		f.cash += value

		th := to.hour(at)
		th.Bought += units
		th.Purchases += value
		fh := f.hour(at)
		fh.Sold += units
		fh.Income += value
	}
}
