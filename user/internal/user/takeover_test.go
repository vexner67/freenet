package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

// takeoverCity is an enterprise of founder, and raider with votes on its
// broker account.
func takeoverCity(t *testing.T) (c *user.City, rules user.FirmRules, f *user.Firm, founder, raider uuid.UUID) {
	t.Helper()

	rules = user.DefaultFirmRules()
	c = newCity()
	founder, raider = uuid.New(), uuid.New()
	f = found(t, c, rules, founder, user.FirmFarm)
	if err := c.Fund(raider, 100_000); err != nil {
		t.Fatal(err)
	}

	return c, rules, f, founder, raider
}

func TestTenderChecks(t *testing.T) {
	t.Parallel()

	c, _, f, founder, raider := takeoverCity(t)
	price := f.Price()

	for _, tc := range []struct {
		name          string
		shares, price int64
		err           error
	}{
		{"no shares", 0, price, user.ErrTender},
		{"more than others hold", 101, price, user.ErrTender},
		{"below the price", 10, price - 1, user.ErrTender},
		{"more than the account", 100, 100_000_000, user.ErrAccount},
	} {
		if _, err := c.Tender(raider, f.ID(), tc.shares, tc.price, cityStart); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.err)
		}
	}

	if _, err := c.Tender(founder, f.ID(), 1, price, cityStart); !errors.Is(err, user.ErrTender) {
		t.Errorf("the only holder offering for its own: %v", err)
	}
	if _, err := c.Tender(raider, uuid.New(), 1, price, cityStart); !errors.Is(err, user.ErrFirmNotFound) {
		t.Errorf("an offer for nothing: %v", err)
	}

	tender, err := c.Tender(raider, f.ID(), 80, price*2, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Account(raider); got != 100_000*100-80*price*2 {
		t.Errorf("the account keeps %d", got)
	}
	if _, err = c.Tender(raider, f.ID(), 10, price*2, cityStart); !errors.Is(err, user.ErrOwnTender) {
		t.Errorf("a second offer: %v", err)
	}
	if !tender.Until.Equal(cityStart.Add(24*time.Hour)) || len(c.Tenders()) != 1 {
		t.Errorf("the offer %+v", tender)
	}
	if news := c.News(); len(news) == 0 || news[0].Kind != user.EventTender || news[0].FirmID != f.ID() {
		t.Errorf("no news of the offer: %+v", news)
	}
}

// TestTakeover: shareholders sell to the offer, and the one who made it
// comes to direct the enterprise.
func TestTakeover(t *testing.T) {
	t.Parallel()

	c, rules, f, founder, raider := takeoverCity(t)
	price := f.Price() * 2
	tender, err := c.Tender(raider, f.ID(), 80, price, cityStart)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = c.SellToTender(rules, raider, tender.ID, 1, cityStart); !errors.Is(err, user.ErrTenderToYourself) {
		t.Errorf("selling to its own offer: %v", err)
	}
	if _, err = c.SellToTender(rules, uuid.New(), tender.ID, 1, cityStart); !errors.Is(err, user.ErrStockShares) {
		t.Errorf("selling shares one has not: %v", err)
	}

	fill, err := c.SellToTender(rules, founder, tender.ID, 40, cityStart)
	if err != nil || fill.Shares != 40 || fill.Money != 40*price {
		t.Fatalf("selling 40: %+v, %v", fill, err)
	}
	if f.DirectorID() != founder {
		t.Errorf("60 to 40, the founder still directs")
	}
	if got, want := c.Account(founder), 40*price-40*price*int64(rules.Fee)/100; got != want {
		t.Errorf("the founder got %d, want %d", got, want)
	}

	if _, err = c.SellToTender(rules, founder, tender.ID, 20, cityStart); err != nil {
		t.Fatal(err)
	}
	if f.DirectorID() != raider {
		t.Fatalf("holding 60 of 100, the raider does not direct")
	}
	if news := c.News(); news[0].Kind != user.EventTakeover || news[0].FirmID != f.ID() {
		t.Errorf("no news of the takeover: %+v", news[0])
	}

	// More than the offer has left sells what it has left.
	fill, err = c.SellToTender(rules, founder, tender.ID, 40, cityStart)
	if err != nil || fill.Shares != 20 || len(c.Tenders()) != 0 {
		t.Errorf("selling past the offer: %+v, %v, %d offers", fill, err, len(c.Tenders()))
	}
	if _, err = c.SellToTender(rules, founder, tender.ID, 1, cityStart); !errors.Is(err, user.ErrTenderNotFound) {
		t.Errorf("selling to an offer done: %v", err)
	}
}

// TestTenderEnds: an offer taken back or past its day gives back the
// money of the shares it did not buy.
func TestTenderEnds(t *testing.T) {
	t.Parallel()

	c, rules, f, founder, raider := takeoverCity(t)
	before := c.Account(raider)
	price := f.Price()

	tender, err := c.Tender(raider, f.ID(), 50, price, cityStart)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.CancelTender(founder, tender.ID); !errors.Is(err, user.ErrTenderNotFound) {
		t.Errorf("another taking it back: %v", err)
	}
	if err = c.CancelTender(raider, tender.ID); err != nil || c.Account(raider) != before || len(c.Tenders()) != 0 {
		t.Fatalf("taking it back: %v, account %d", err, c.Account(raider))
	}

	if tender, err = c.Tender(raider, f.ID(), 50, price, cityStart); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SellToTender(rules, founder, tender.ID, 10, cityStart); err != nil {
		t.Fatal(err)
	}
	c.Tick(rules, cityStart.Add(25*time.Hour))
	if len(c.Tenders()) != 0 || c.Account(raider) != before-10*price {
		t.Errorf("past its day: %d offers, account %d", len(c.Tenders()), c.Account(raider))
	}
}

// TestTenderOfClosed: an enterprise closed gives back the money of the
// offers for it.
func TestTenderOfClosed(t *testing.T) {
	t.Parallel()

	c, rules, f, founder, raider := takeoverCity(t)
	before := c.Account(raider)
	if _, err := c.Tender(raider, f.ID(), 50, f.Price(), cityStart); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Liquidate(rules, founder, f.ID()); err != nil {
		t.Fatal(err)
	}
	if len(c.Tenders()) != 0 || c.Account(raider) != before {
		t.Errorf("after closing: %d offers, account %d", len(c.Tenders()), c.Account(raider))
	}

	restored := user.RestoreCity(c.State())
	if len(restored.Tenders()) != 0 || len(restored.Holders()) != 1 {
		t.Errorf("restored: %+v", restored.Holders())
	}
}
