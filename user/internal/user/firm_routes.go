package user

import (
	"errors"
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

// MaxRoutes are the most supplies an enterprise keeps.
const MaxRoutes = 5

var (
	ErrRoute         = errors.New("a supply goes to another enterprise you direct that works with the good, of 1 unit or more an hour, once from 0 units are kept")
	ErrTooManyRoutes = errors.New("an enterprise keeps at most 5 supplies")
	ErrRouteNotFound = errors.New("there is no such supply")
)

// SupplyRoute is a supply of an enterprise: Units of Good an hour to the
// enterprise To, once its storage keeps From units or more.
type SupplyRoute struct {
	ID    uuid.UUID
	To    uuid.UUID
	Good  Good
	From  int64
	Units int64
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

	for _, r := range f.routes {
		to, _ := c.Firm(r.To) // kept above
		if to.directorID != f.directorID || f.stock[r.Good] < r.From {
			continue
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
