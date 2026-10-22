package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestGiftTraits(t *testing.T) {
	t.Parallel()

	for _, list := range [][]user.GiftTrait{user.GiftModels(), user.GiftBackdrops(), user.GiftSymbols()} {
		sum := 0
		for _, tr := range list {
			sum += tr.Weight
		}
		if sum != user.TraitWeights {
			t.Fatalf("weights of %v add up to %d", list, sum)
		}
	}

	// The first roll falls on the commonest, the last on the rarest.
	for _, r := range []struct {
		roll int64
		want user.GiftTraits
	}{
		{0, user.GiftTraits{Model: "classic", Backdrop: "sky", Symbol: "f"}},
		{user.TraitWeights - 1, user.GiftTraits{Model: "amethyst", Backdrop: "black", Symbol: "rocket"}},
	} {
		got, err := user.RollGiftTraits(func(int64) (int64, error) { return r.roll, nil })
		if err != nil || got != r.want {
			t.Fatalf("RollGiftTraits(%d) = %+v, %v", r.roll, got, err)
		}
	}

	failed := errors.New("no randomness")
	if _, err := user.RollGiftTraits(func(int64) (int64, error) { return 0, failed }); !errors.Is(err, failed) {
		t.Fatalf("RollGiftTraits with a broken roll: %v", err)
	}

	if _, err := user.NewGiftTraits("classic", "sky", "nope"); !errors.Is(err, user.ErrUnknownTrait) {
		t.Fatalf("NewGiftTraits of an unknown symbol: %v", err)
	}
	if got, err := user.NewTraitFilter(user.GiftModels(), ""); err != nil || got != "" {
		t.Fatalf("NewTraitFilter of any = %q, %v", got, err)
	}
	if _, err := user.NewTraitFilter(user.GiftModels(), "sky"); !errors.Is(err, user.ErrUnknownTrait) {
		t.Fatalf("NewTraitFilter of a backdrop among models: %v", err)
	}
}

func TestCollectibleGift(t *testing.T) {
	t.Parallel()

	ann := newUser(t)
	bob := newUser(t)

	g, err := ann.GiftFrom(bob.ID(), user.Ties{}, "freenet_coin", "Привет", true)
	if err != nil {
		t.Fatal(err)
	}

	traits := user.GiftTraits{Model: "onyx", Backdrop: "gold", Symbol: "star"}

	if _, err = g.Upgraded(bob.ID(), 1, traits); !errors.Is(err, user.ErrNotGiftOwner) {
		t.Fatalf("Upgraded by the sender: %v", err)
	}
	if err = g.CheckTrade(ann.ID()); !errors.Is(err, user.ErrNotCollectible) {
		t.Fatalf("CheckTrade of a plain gift: %v", err)
	}

	c, err := g.Upgraded(ann.ID(), 7, traits)
	if err != nil || c.Number() != 7 || c.Traits() != traits || c.Caption() != "Привет" || !c.IsCollectible() {
		t.Fatalf("Upgraded = %+v, %v", c, err)
	}
	if _, err = c.Upgraded(ann.ID(), 8, traits); !errors.Is(err, user.ErrGiftCollectible) {
		t.Fatalf("Upgraded twice: %v", err)
	}

	for _, price := range []int64{-1, user.MaxGiftSalePrice + 1} {
		if _, err = c.OnSale(ann.ID(), price); !errors.Is(err, user.ErrGiftSalePrice) {
			t.Fatalf("OnSale(%d): %v", price, err)
		}
	}

	sale, err := c.OnSale(ann.ID(), 100)
	if err != nil || sale.SalePrice() != 100 {
		t.Fatalf("OnSale = %+v, %v", sale, err)
	}
	if err = sale.CheckTrade(ann.ID()); !errors.Is(err, user.ErrGiftOnSale) {
		t.Fatalf("CheckTrade on sale: %v", err)
	}

	for _, tc := range []struct {
		name  string
		g     *user.Gift
		buyer uuid.UUID
		price int64
		want  error
	}{
		{"not on sale", c, bob.ID(), 100, user.ErrGiftNotOnSale},
		{"own", sale, ann.ID(), 100, user.ErrOwnGift},
		{"price changed", sale, bob.ID(), 90, user.ErrGiftPriceChanged},
		{"ok", sale, bob.ID(), 100, nil},
	} {
		if err = tc.g.CheckBuy(tc.buyer, tc.price); !errors.Is(err, tc.want) {
			t.Fatalf("CheckBuy %s: %v, want %v", tc.name, err, tc.want)
		}
	}

	passed := sale.PassedTo(bob.ID())
	if passed.OwnerID() != bob.ID() || passed.SenderID() != ann.ID() || passed.Caption() != "" || passed.Anonymous() || passed.IsOnSale() || passed.Number() != 7 {
		t.Fatalf("PassedTo = %+v", passed)
	}

	minted, err := user.MintedGift("freenet_crown", 3, traits)
	if err != nil || !minted.IsHeldBySite() || minted.CheckOwner(uuid.Nil) == nil {
		t.Fatalf("MintedGift = %+v, %v", minted, err)
	}
}

func TestGiftFee(t *testing.T) {
	t.Parallel()

	u := newUser(t)
	now := time.Now()

	for _, tc := range []struct{ price, fee int64 }{{1, 1}, {19, 1}, {100, 5}, {1000, 50}, {1001, 51}} {
		if got := u.GiftFeeAt(tc.price, now); got != tc.fee {
			t.Fatalf("GiftFeeAt(%d) = %d, want %d", tc.price, got, tc.fee)
		}
	}
}

func TestGiftStockAndSupply(t *testing.T) {
	t.Parallel()

	if left := (user.GiftStock{}).Left(); left != -1 {
		t.Fatalf("Left without an edition = %d", left)
	}
	if left := (user.GiftStock{Supply: 5, Sold: 7}).Left(); left != 0 {
		t.Fatalf("Left past the edition = %d", left)
	}

	for _, n := range []int64{-1, user.MaxGiftSupply + 1} {
		if _, err := user.NewGiftSupply(n); !errors.Is(err, user.ErrGiftSupplyInvalid) {
			t.Fatalf("NewGiftSupply(%d): %v", n, err)
		}
	}

	for _, n := range []int{0, user.MaxAuctionHours + 1} {
		if _, err := user.NewAuctionHours(n); !errors.Is(err, user.ErrAuctionHours) {
			t.Fatalf("NewAuctionHours(%d): %v", n, err)
		}
	}

	if _, err := user.NewMarketSort("cheap"); !errors.Is(err, user.ErrMarketSort) {
		t.Fatalf("NewMarketSort of nothing: %v", err)
	}
	if s, err := user.NewMarketSort(""); err != nil || s != user.MarketCheapest {
		t.Fatalf("NewMarketSort default = %q, %v", s, err)
	}
}

func TestGiftAuction(t *testing.T) {
	t.Parallel()

	seller, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

	if _, err := user.NewGiftAuction(uuid.New(), seller, 0, 1, now); !errors.Is(err, user.ErrGiftSalePrice) {
		t.Fatalf("NewGiftAuction from 0: %v", err)
	}

	a, err := user.NewGiftAuction(uuid.New(), seller, 100, 1, now)
	if err != nil || a.MinBid() != 100 || !a.EndsAt().Equal(now.Add(time.Hour)) || a.CheckCancel() != nil {
		t.Fatalf("NewGiftAuction = %+v, %v", a, err)
	}

	if _, err = a.Bid(seller, 100, now); !errors.Is(err, user.ErrOwnGift) {
		t.Fatalf("Bid of the seller: %v", err)
	}
	if _, err = a.Bid(ann, 99, now); !errors.Is(err, user.ErrBidTooLow) {
		t.Fatalf("Bid under the start: %v", err)
	}

	b, err := a.Bid(ann, 100, now)
	if err != nil || b.TopBid() != 100 || b.BidderID() != ann || b.Bids() != 1 || b.MinBid() != 105 {
		t.Fatalf("Bid = %+v, %v", b, err)
	}
	if err = b.CheckCancel(); !errors.Is(err, user.ErrAuctionHasBids) {
		t.Fatalf("CheckCancel with a bid: %v", err)
	}
	if _, err = b.Bid(bob, 104, now); !errors.Is(err, user.ErrBidTooLow) {
		t.Fatalf("Bid under the step: %v", err)
	}

	// A bid in the last minutes moves the end on.
	late := b.EndsAt().Add(-time.Minute)
	c, err := b.Bid(bob, 105, late)
	if err != nil || !c.EndsAt().Equal(late.Add(user.AuctionExtension)) {
		t.Fatalf("late Bid = %+v, %v", c, err)
	}

	if _, err = c.Bid(ann, 200, c.EndsAt()); !errors.Is(err, user.ErrAuctionEnded) {
		t.Fatalf("Bid after the end: %v", err)
	}
	if _, err = c.Bid(ann, user.MaxGiftSalePrice+1, late); !errors.Is(err, user.ErrGiftSalePrice) {
		t.Fatalf("Bid over the most: %v", err)
	}
}

func newUser(t *testing.T) *user.User {
	t.Helper()

	u, err := user.New("someone@example.com")
	if err != nil {
		t.Fatal(err)
	}

	return u
}
