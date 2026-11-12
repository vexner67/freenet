package user

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrGamesClosed     = errors.New("games are closed")
	ErrClosingOverflow = errors.New("game settlement exceeds the supported balance")
)

type GamePayout struct {
	UserID uuid.UUID
	Reason string
	Votes  int64
}

type ClosingMiner struct {
	UserID   uuid.UUID
	Backpack map[Ore]int64
}

type ClosingPet struct {
	UserID   uuid.UUID
	Kind     PetKind
	Wardrobe []PetItem
}

// ClosingAssets holds the game property at one point in time.
type ClosingAssets struct {
	City    *City
	Rules   FirmRules
	Tables  []*HoldemTable
	Stakes  []Payout
	Rockets []*RocketRound
	Poker   []Payout
	Mine    Mine
	Miners  []ClosingMiner
	Shop    PetShop
	Pets    []ClosingPet
}

// ClosingPayouts prices the frozen game property without changing it.
func (a ClosingAssets) ClosingPayouts(at time.Time) ([]GamePayout, error) {
	payouts, err := a.City.closingCity(a.Rules)
	if err != nil {
		return nil, err
	}
	for _, table := range a.Tables {
		for _, seat := range table.Seats() {
			chips := seat.Stack
			if table.Street().Betting() {
				chips += seat.Put
			}
			payouts = append(payouts, GamePayout{seat.UserID, "table", chips})
		}
	}
	for _, stake := range a.Stakes {
		payouts = append(payouts, GamePayout{stake.UserID, "stake", stake.Votes})
	}
	for _, hand := range a.Poker {
		payouts = append(payouts, GamePayout{hand.UserID, "stake", hand.Votes})
	}
	for _, round := range a.Rockets {
		if round.StateAt(at) == RocketFlying {
			payouts = append(payouts, GamePayout{round.UserID(), "stake", round.Amount()})
		}
	}
	return a.closingInventory(payouts)
}

func (a ClosingAssets) closingInventory(payouts []GamePayout) ([]GamePayout, error) {
	for _, miner := range a.Miners {
		for ore, units := range miner.Backpack {
			vein, err := a.Mine.Vein(ore)
			if err != nil {
				return nil, err
			}
			payouts = append(payouts, GamePayout{miner.UserID, "ore", vein.SaleValue(units)})
		}
	}
	for _, pet := range a.Pets {
		price, err := a.Shop.Price(pet.Kind)
		if errors.Is(err, ErrNotInPetShop) {
			price = pet.Kind.Rarity().MinPrice()
		} else if err != nil {
			return nil, err
		}
		payouts = append(payouts, GamePayout{pet.UserID, "pet", price})
		for _, item := range pet.Wardrobe {
			payouts = append(payouts, GamePayout{pet.UserID, "clothes", item.Spec().Price})
		}
	}

	return payouts, nil
}

func (c *City) closingCity(rules FirmRules) ([]GamePayout, error) {
	broker, err := c.closingBroker()
	if err != nil {
		return nil, err
	}

	payouts := make([]GamePayout, 0, len(broker))
	for id, hundredths := range broker {
		payouts = append(payouts, GamePayout{id, "broker", hundredths / 100})
	}
	for _, firm := range c.Firms() {
		worth, err := c.closingWorth(rules, firm)
		if err != nil {
			return nil, err
		}
		shares, _ := c.share(firm.id, worth/100)
		for _, share := range shares {
			payouts = append(payouts, GamePayout{share.UserID, "firm", share.Votes})
		}
	}

	return payouts, nil
}

// closingWorth returns all the investment when the site closes the game.
func (c *City) closingWorth(rules FirmRules, firm *Firm) (int64, error) {
	plan, err := rules.Plan(firm.kind)
	if err != nil {
		return 0, err
	}
	invested, err := AddClosingVotes(plan.Invested(firm.level), firm.improved)
	if err != nil || invested > math.MaxInt64/100 {
		return 0, ErrClosingOverflow
	}
	cash, err := AddClosingVotes(firm.cash, c.Escrow(firm.id))
	if err != nil {
		return 0, err
	}

	return AddClosingVotes(invested*100, cash)
}

func (c *City) closingBroker() (map[uuid.UUID]int64, error) {
	cash := make(map[uuid.UUID]int64, len(c.accounts))
	add := func(id uuid.UUID, amount int64) error {
		total, err := AddClosingVotes(cash[id], amount)
		cash[id] = total
		return err
	}
	for _, account := range c.accounts {
		if err := add(account.UserID, account.Cash); err != nil {
			return nil, err
		}
	}
	for _, order := range c.stockOrders {
		if order.Buy {
			if err := add(order.UserID, order.Price*order.Shares); err != nil {
				return nil, err
			}
		}
	}
	for _, offer := range c.tenders {
		if err := add(offer.UserID, offer.Price*offer.Shares); err != nil {
			return nil, err
		}
	}

	return cash, nil
}

// AddClosingVotes checks totals before a settlement can be recorded.
func AddClosingVotes(total, amount int64) (int64, error) {
	if total < 0 || amount < 0 || amount > math.MaxInt64-total {
		return 0, ErrClosingOverflow
	}

	return total + amount, nil
}
