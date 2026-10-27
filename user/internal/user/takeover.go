package user

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// A takeover: a page offers the shareholders of an enterprise to buy
// their shares at a price of its own, for a day, the money of the offer
// kept on its broker account. Each shareholder sells to it as much as it
// likes; whoever comes to hold the most shares directs the enterprise.

const (
	// tenderTerm is how long an offer waits for the shareholders.
	tenderTerm = 24 * time.Hour
	// MaxTenders are the most offers a page makes at a time.
	MaxTenders = 5
)

var (
	ErrTender           = errors.New("an offer must be for 1 share or more of those others hold, at no less than a share goes for now")
	ErrTenderNotFound   = errors.New("there is no such offer")
	ErrOwnTender        = errors.New("you make an offer for this enterprise already")
	ErrTooManyTenders   = errors.New("a page makes at most 5 offers at a time")
	ErrTenderToYourself = errors.New("you cannot sell to your own offer")
)

// Tender is an offer of UserID to buy Shares of an enterprise at Price
// hundredths a share till Until; the money of the shares left to buy is
// kept.
type Tender struct {
	ID       uuid.UUID
	FirmID   uuid.UUID
	UserID   uuid.UUID
	Price    int64
	Shares   int64
	PlacedAt time.Time
	Until    time.Time
}

// Tenders are the offers to buy out shareholders, the oldest first.
func (c *City) Tenders() []Tender {
	return append([]Tender(nil), c.tenders...)
}

// Tender offers the shareholders of the enterprise to buy shares at price
// hundredths a share, paid of the account of userID as they sell; no
// fewer than a share goes for now, and no more shares than others hold.
// ErrTender, ErrOwnTender, ErrTooManyTenders, ErrAccount.
func (c *City) Tender(userID, firmID uuid.UUID, shares, price int64, now time.Time) (Tender, error) {
	f, err := c.Firm(firmID)
	if err != nil {
		return Tender{}, err
	}

	others := c.humanShares(firmID) - c.held(firmID, userID)
	if shares < 1 || shares > others || price < max(f.price, 1) || price > MaxFirmPrice {
		return Tender{}, ErrTender
	}

	made := 0
	for _, t := range c.tenders {
		if t.UserID != userID {
			continue
		}
		if t.FirmID == firmID {
			return Tender{}, ErrOwnTender
		}
		made++
	}
	if made >= MaxTenders {
		return Tender{}, ErrTooManyTenders
	}

	if c.Account(userID) < shares*price {
		return Tender{}, ErrAccount
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Tender{}, err
	}

	t := Tender{ID: id, FirmID: firmID, UserID: userID, Price: price, Shares: shares, PlacedAt: now.UTC(), Until: now.UTC().Add(tenderTerm)}
	c.addCash(userID, -shares*price)
	c.tenders = append(c.tenders, t)

	e := c.begin(tenderEvent, firmID, now.UTC())
	e.End = t.Until
	c.events[len(c.events)-1] = e
	f.mood = min(f.mood+tenderEvent.mood, maxMood)

	return t, nil
}

func (c *City) tender(id uuid.UUID) (int, error) {
	i := slices.IndexFunc(c.tenders, func(t Tender) bool { return t.ID == id })
	if i < 0 {
		return 0, ErrTenderNotFound
	}

	return i, nil
}

// SellToTender sells up to shares of userID free of its orders to the
// offer at its price, the fee of the exchange paid by the seller; who
// holds the most after directs the enterprise. ErrTenderNotFound,
// ErrTenderToYourself, ErrStockOrder, ErrStockShares.
func (c *City) SellToTender(rules FirmRules, userID, tenderID uuid.UUID, shares int64, now time.Time) (StockFill, error) {
	i, err := c.tender(tenderID)
	if err != nil {
		return StockFill{}, err
	}

	t := &c.tenders[i]
	f, err := c.Firm(t.FirmID)
	if err != nil {
		return StockFill{}, ErrTenderNotFound
	}

	if t.UserID == userID {
		return StockFill{}, ErrTenderToYourself
	}
	if shares < 1 {
		return StockFill{}, ErrStockOrder
	}

	units := min(shares, t.Shares)
	if c.Free(f.id, userID) < units {
		return StockFill{}, ErrStockShares
	}

	value := units * t.Price
	fee := value * int64(rules.Fee) / 100
	buyer, director := t.UserID, f.directorID

	c.setHolding(f.id, userID, c.held(f.id, userID)-units)
	c.setHolding(f.id, buyer, c.held(f.id, buyer)+units)
	c.addCash(userID, value-fee)
	c.stats.Fees += fee
	c.trade(f, now, t.Price, units)

	if t.Shares -= units; t.Shares == 0 {
		c.tenders = slices.Delete(c.tenders, i, i+1)
	}

	c.afterDeal(f)
	if f.directorID == buyer && director != buyer {
		c.begin(takeoverEvent, f.id, now.UTC())
	}

	return StockFill{Shares: units, Money: value}, nil
}

// CancelTender takes the offer of userID back with the money of the shares
// it did not buy; ErrTenderNotFound.
func (c *City) CancelTender(userID, tenderID uuid.UUID) error {
	i, err := c.tender(tenderID)
	if err != nil || c.tenders[i].UserID != userID {
		return ErrTenderNotFound
	}

	c.closeTender(i)

	return nil
}

// closeTender drops the offer i, its money back to the account.
func (c *City) closeTender(i int) {
	t := c.tenders[i]
	c.addCash(t.UserID, t.Shares*t.Price)
	c.tenders = slices.Delete(c.tenders, i, i+1)
}

// closeTenders closes the offers past their day by the end of the hour at
// and those of enterprises gone.
func (c *City) closeTenders(at time.Time) {
	for i := len(c.tenders) - 1; i >= 0; i-- {
		t := c.tenders[i]
		if _, err := c.Firm(t.FirmID); err != nil || !at.Before(t.Until) {
			c.closeTender(i)
		}
	}
}
