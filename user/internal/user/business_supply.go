package user

import (
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// Order is supplies on the way: they come at Arrives, from the till when
// a contract ordered them.
type Order struct {
	ID       int
	Supply   Supply
	Supplier Supplier
	Units    int64
	Arrives  time.Time
	FromTill bool
}

// Contract is a standing order: while the supply in the storage and on
// the way is below Below, the buyer orders Units of it from Supplier,
// paying from the till. Stalled is told once when the till cannot pay.
type Contract struct {
	Supply   Supply
	Supplier Supplier
	Below    int64
	Units    int64
	Stalled  bool
}

// storekeeperLoss is the part of a delivery lost without a storekeeper.
const storekeeperLoss = 0.1

// UnitCost is what a unit of a supply costs b from a supplier, in
// hundredths of a vote: a buyer bargains two percent a skill off.
func (b Business) UnitCost(spec SupplySpec, sup SupplierSpec, r BusinessRules) float64 {
	return float64(spec.Cost) * float64(sup.Percent) / 100 * float64(r.Goods()) / 100 *
		(1 - 0.02*float64(b.skillOf(ProfBuyer)))
}

// OrderCost is what units of a supply cost b in whole votes, rounded up.
func (b Business) OrderCost(spec SupplySpec, sup SupplierSpec, r BusinessRules, units int64) int64 {
	return int64(math.Ceil(float64(units)*b.UnitCost(spec, sup, r)/100 - 1e-9))
}

// OnTheWay is the units of a supply ordered and not come yet.
func (b Business) OnTheWay(s Supply) int64 {
	var units int64
	for _, o := range b.orders {
		if o.Supply == s {
			units += o.Units
		}
	}

	return units
}

func (b Business) supplyAndSupplier(s Supply, supplier Supplier) (SupplySpec, SupplierSpec, error) {
	spec, err := BusinessCatalog(b.kind)
	if err != nil {
		return SupplySpec{}, SupplierSpec{}, err
	}

	ss, err := spec.Supply(s)
	if err != nil {
		return SupplySpec{}, SupplierSpec{}, err
	}

	sup, err := SupplierCatalog(supplier)
	if err != nil {
		return SupplySpec{}, SupplierSpec{}, err
	}

	return ss, sup, nil
}

// Ordered is b with units of a supply ordered from a supplier for the
// votes of the owner, and what they cost.
func (b Business) Ordered(r BusinessRules, s Supply, supplier Supplier, units int64, now time.Time) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}

	ss, sup, err := b.supplyAndSupplier(s, supplier)
	if err != nil {
		return Business{}, 0, err
	}
	if units < 1 || float64(units)+b.stock[s]+float64(b.OnTheWay(s)) > float64(b.StorageOf(ss)) {
		return Business{}, 0, ErrBusinessOrder
	}

	cost := b.OrderCost(ss, sup, r, units)
	b = b.clone()
	b.order(s, sup, units, now.UTC(), false)

	return b, cost, nil
}

func (b *Business) order(s Supply, sup SupplierSpec, units int64, at time.Time, fromTill bool) {
	b.orders = append(b.orders, Order{
		ID: b.nextID, Supply: s, Supplier: sup.Supplier, Units: units, FromTill: fromTill,
		Arrives: at.Add(time.Duration(sup.Hours * float64(time.Hour))),
	})
	b.nextID++
}

// Contracted is b with the contract of a supply set, or ended with no
// units.
func (b Business) Contracted(s Supply, supplier Supplier, below, units int64) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}

	b = b.clone()
	b.contracts = slices.DeleteFunc(b.contracts, func(c Contract) bool { return c.Supply == s })
	if units == 0 {
		return b, nil
	}

	ss, _, err := b.supplyAndSupplier(s, supplier)
	if err != nil {
		return Business{}, err
	}

	storage := b.StorageOf(ss)
	if units < 1 || units > storage || below < 1 || below > storage {
		return Business{}, ErrBusinessContract
	}

	b.contracts = append(b.contracts, Contract{Supply: s, Supplier: supplier, Below: below, Units: units})
	slices.SortFunc(b.contracts, func(x, y Contract) int { return supplyIndex(x.Supply) - supplyIndex(y.Supply) })

	return b, nil
}

func supplyIndex(s Supply) int {
	for _, k := range kindSpecs {
		for i, spec := range k.Supplies {
			if spec.Supply == s {
				return i
			}
		}
	}

	return 0
}

// deliver brings the orders come by at into the storage: the wholesaler
// may bring less, a business without a storekeeper loses a tenth, and
// what does not fit is lost.
func (b *Business) deliver(spec KindSpec, at time.Time) {
	keeper := b.skillOf(ProfStorekeeper) > 0

	b.orders = slices.DeleteFunc(b.orders, func(o Order) bool {
		if o.Arrives.After(at) {
			return false
		}

		ss, err := spec.Supply(o.Supply)
		if err != nil {
			return true
		}

		got := float64(o.Units)
		if sup, err := SupplierCatalog(o.Supplier); err == nil && sup.Short > 0 {
			rng := rand.New(rand.NewPCG(b.seed, uint64(o.ID))) //nolint:gosec // the game, not secrets
			got *= 1 - float64(rng.IntN(sup.Short+1))/100
		}
		if !keeper {
			got *= 1 - storekeeperLoss
		}

		before := b.stock[o.Supply]
		b.stock[o.Supply] = min(before+got, float64(b.StorageOf(ss)))
		b.note(Note{
			At: o.Arrives, Kind: NoteDelivery, Subject: string(o.Supply),
			Amount: b.stock[o.Supply] - before, Money: float64(o.Units) - (b.stock[o.Supply] - before),
		})

		return true
	})
}

// runContracts orders, from the till, the supplies of the contracts that
// run low: only a buyer runs them.
func (b *Business) runContracts(spec KindSpec, r BusinessRules, at time.Time, hour *BusinessHour) {
	if b.skillOf(ProfBuyer) == 0 {
		return
	}

	for i, c := range b.contracts {
		ss, err := spec.Supply(c.Supply)
		if err != nil {
			continue
		}
		sup, err := SupplierCatalog(c.Supplier)
		if err != nil {
			continue
		}

		have := b.stock[c.Supply] + float64(b.OnTheWay(c.Supply))
		units := min(c.Units, b.StorageOf(ss)-int64(math.Ceil(have)))
		if have >= float64(c.Below) || units < 1 {
			continue
		}

		cost := float64(units) * b.UnitCost(ss, sup, r)
		if b.cash < cost {
			if !c.Stalled {
				b.contracts[i].Stalled = true
				b.note(Note{At: at, Kind: NoteUnpaid, Subject: string(c.Supply)})
			}

			continue
		}

		b.cash -= cost
		hour.Supplies += cost
		b.contracts[i].Stalled = false
		b.order(c.Supply, sup, units, at, true)
		b.note(Note{At: at, Kind: NoteOrdered, Subject: string(c.Supply), Amount: float64(units), Money: cost})
	}
}
