package user

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	// maxOpenContracts are the most orders of the city nobody took yet.
	maxOpenContracts = 4
	// contractChance is the chance the city orders a good in an hour.
	contractChance = 0.35
	// contractOpen is how long an order waits to be taken, contractTerm
	// how long a taker has to deliver it.
	contractOpen = 12 * time.Hour
	contractTerm = 24 * time.Hour
	// maxTakenContracts are the most orders an enterprise works on.
	maxTakenContracts = 2
	// contractPenalty is the percent of what is left undelivered an
	// enterprise pays when the term ends.
	contractPenalty = 10
)

var (
	ErrContractNotFound = errors.New("there is no such order of the city")
	ErrContractTaken    = errors.New("the order is taken")
	ErrTooManyContracts = errors.New("an enterprise works on at most 2 orders of the city")
	ErrDeliver          = errors.New("deliver from 1 unit to what is left of the order")
)

// Contract is an order of the city: Units of a good at Price hundredths a
// unit, waiting to be taken from Posted; the enterprise FirmID took it and
// delivers it by Deadline, paid for every unit delivered, and pays a fine
// for what it did not.
type Contract struct {
	ID        uuid.UUID
	Good      Good
	Units     int64
	Price     int64
	Posted    time.Time
	Deadline  time.Time
	FirmID    uuid.UUID
	Delivered int64
}

// IsTaken is whether an enterprise took the order.
func (k Contract) IsTaken() bool {
	return k.FirmID != uuid.Nil
}

// Contracts are the orders of the city, the oldest first.
func (c *City) Contracts() []Contract {
	return append([]Contract(nil), c.contracts...)
}

func (c *City) contract(id uuid.UUID) (int, error) {
	i := slices.IndexFunc(c.contracts, func(k Contract) bool { return k.ID == id })
	if i < 0 {
		return 0, ErrContractNotFound
	}

	return i, nil
}

// Take takes the order for the enterprise: it must keep the good;
// ErrContractNotFound, ErrContractTaken, ErrFirmGood, ErrTooManyContracts.
func (c *City) Take(userID, firmID, contractID uuid.UUID, now time.Time) error {
	f, err := c.directed(userID, firmID)
	if err != nil {
		return err
	}

	i, err := c.contract(contractID)
	if err != nil {
		return err
	}

	k := &c.contracts[i]
	if k.IsTaken() {
		return ErrContractTaken
	}
	if !f.spec().Keeps(k.Good) {
		return ErrFirmGood
	}

	taken := 0
	for _, other := range c.contracts {
		if other.FirmID == firmID {
			taken++
		}
	}
	if taken >= maxTakenContracts {
		return ErrTooManyContracts
	}

	k.FirmID, k.Deadline = firmID, now.UTC().Add(contractTerm)

	return nil
}

// Deliver delivers units of the storage on the order the enterprise took,
// paid into its cash; the order delivered in full is done.
// ErrContractNotFound, ErrDeliver, ErrFirmStock.
func (c *City) Deliver(userID, contractID uuid.UUID, units int64, now time.Time) (int64, error) {
	i, err := c.contract(contractID)
	if err != nil {
		return 0, err
	}

	k := &c.contracts[i]
	if !k.IsTaken() {
		return 0, ErrContractNotFound
	}

	f, err := c.directed(userID, k.FirmID)
	if err != nil {
		return 0, ErrContractNotFound
	}

	if units < 1 || units > k.Units-k.Delivered {
		return 0, ErrDeliver
	}
	if f.stock[k.Good] < units {
		return 0, ErrFirmStock
	}

	got := units * k.Price
	f.stock[k.Good] -= units
	f.cash += got
	h := f.hour(now)
	h.Sold += units
	h.Income += got

	if k.Delivered += units; k.Delivered == k.Units {
		c.contracts = slices.Delete(c.contracts, i, i+1)
	}

	return got, nil
}

// order posts an order of the city in the hour at, now and then: a good
// at a third to a half above its price, as much as a first level makes
// of it in 8 to 24 hours.
func (c *City) order(rules FirmRules, at time.Time) {
	open := 0
	for _, k := range c.contracts {
		if !k.IsTaken() {
			open++
		}
	}
	if open >= maxOpenContracts || c.luck(at, drawContract, uuid.Nil) >= contractChance {
		return
	}

	g := Goods[int(c.luck(at, drawContractGood, uuid.Nil)*float64(len(Goods)))%len(Goods)]
	plan, err := rules.Good(g)
	if err != nil {
		return
	}

	var rate float64
	var staff int
	for _, spec := range firmSpecs {
		if spec.Output == g {
			p, err := rules.Plan(spec.Kind)
			if err != nil {
				return
			}
			rate, staff = spec.Rate[g], p.Staff
		}
	}
	if rate == 0 {
		return
	}

	hours := 8 + 16*c.luck(at, drawContractSize, uuid.Nil)
	percent := 130 + 30*c.luck(at, drawContractPrice, uuid.Nil)

	id, err := uuid.NewV7()
	if err != nil {
		return
	}

	c.contracts = append(c.contracts, Contract{
		ID: id, Good: g, Units: max(int64(math.Ceil(rate*float64(staff)*hours)), 1),
		Price: max(int64(float64(plan.Base)*percent/100), 1), Posted: at, Deadline: at.Add(contractOpen),
	})
}

// expire closes the orders whose time ended by the hour at: those not
// taken go, and the takers pay the fine for what is left.
func (c *City) expire(at time.Time) {
	c.contracts = slices.DeleteFunc(c.contracts, func(k Contract) bool {
		if at.Before(k.Deadline) {
			return false
		}

		if f, err := c.Firm(k.FirmID); err == nil {
			f.cash = max(f.cash-(k.Units-k.Delivered)*k.Price*contractPenalty/100, 0)
		}

		return true
	})
}
