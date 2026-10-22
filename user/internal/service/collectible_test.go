package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func (w giftWorld) votes(t *testing.T, id uuid.UUID) int64 {
	t.Helper()

	u, err := w.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	return u.Wallet().Votes
}

func TestGiftService_Collectibles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newGiftWorld()
	w.gifts.WithRoll(func(int64) (int64, error) { return 999, nil })
	ann, bob, cat := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	sent, err := w.gifts.Send(ctx, bob, ann, giftDraft("freenet_coin"))
	if err != nil {
		t.Fatal(err)
	}
	annVotes := w.votes(t, ann)

	if _, err = w.gifts.Upgrade(ctx, bob, sent.ID); !errors.Is(err, user.ErrNotGiftOwner) {
		t.Fatalf("the sender upgrades the gift: %v", err)
	}

	upgraded, err := w.gifts.Upgrade(ctx, ann, sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 999 falls on the rarest of every list.
	want := user.GiftTraits{Model: "amethyst", Backdrop: "black", Symbol: "rocket"}
	if upgraded.Number != 1 || upgraded.Traits != want || upgraded.Caption != sent.Caption {
		t.Fatalf("upgraded = %+v", upgraded)
	}
	if got := w.votes(t, ann); got != annVotes-user.UpgradePrice {
		t.Fatalf("the upgrade cost %d", annVotes-got)
	}
	if _, err = w.gifts.Upgrade(ctx, ann, sent.ID); !errors.Is(err, user.ErrGiftCollectible) {
		t.Fatalf("upgraded twice: %v", err)
	}

	// On sale: in the market, not deleted, bought at the price shown.
	if _, err = w.gifts.SetSale(ctx, ann, sent.ID, 100); err != nil {
		t.Fatal(err)
	}
	market, err := w.gifts.Market(ctx, bob, user.MarketFilter{Sort: user.MarketCheapest}, "", 10)
	if err != nil || len(market.Items) != 1 || market.Items[0].Owner == nil || market.Items[0].Owner.ID != ann {
		t.Fatalf("market = %+v, %v", market, err)
	}
	if err = w.gifts.Delete(ctx, ann, sent.ID); !errors.Is(err, user.ErrGiftOnSale) {
		t.Fatalf("a gift on sale deleted: %v", err)
	}
	if _, err = w.gifts.Buy(ctx, ann, sent.ID, 100); !errors.Is(err, user.ErrOwnGift) {
		t.Fatalf("one's own gift bought: %v", err)
	}
	if _, err = w.gifts.Buy(ctx, bob, sent.ID, 99); !errors.Is(err, user.ErrGiftPriceChanged) {
		t.Fatalf("bought for another price: %v", err)
	}

	annVotes, bobVotes := w.votes(t, ann), w.votes(t, bob)
	w.notifier.sent, w.notifier.deleted = nil, nil

	balance, err := w.gifts.Buy(ctx, bob, sent.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if balance != bobVotes-100 || w.votes(t, ann) != annVotes+100-5 {
		t.Fatalf("after the sale bob has %d, ann %d (had %d)", balance, w.votes(t, ann), annVotes)
	}
	if len(w.notifier.deleted) != 1 || w.notifier.deleted[0].recipient != ann ||
		len(w.notifier.sent) != 1 || w.notifier.sent[0] != (giftNote{sent.ID, ann, bob}) {
		t.Fatalf("notifications: sent %+v, deleted %+v", w.notifier.sent, w.notifier.deleted)
	}

	// Given away: for a price, never to oneself.
	if err = w.gifts.Transfer(ctx, bob, bob, sent.ID); !errors.Is(err, user.ErrOwnGift) {
		t.Fatalf("given to oneself: %v", err)
	}
	if err = w.gifts.Transfer(ctx, ann, cat, sent.ID); !errors.Is(err, user.ErrNotGiftOwner) {
		t.Fatalf("the old owner gives it away: %v", err)
	}
	bobVotes = w.votes(t, bob)
	if err = w.gifts.Transfer(ctx, bob, cat, sent.ID); err != nil {
		t.Fatal(err)
	}
	if w.votes(t, bob) != bobVotes-user.GiftTransferPrice {
		t.Fatal("the transfer is free")
	}

	got, err := w.gifts.Get(ctx, ann, sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner == nil || got.Owner.ID != cat || len(got.History) != 3 ||
		got.History[0].Kind != user.GiftEventTransfer || got.History[1].Price != 100 || got.History[2].Kind != user.GiftEventUpgrade {
		t.Fatalf("collectible = %+v", got)
	}
}

func TestGiftService_Auctions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newGiftWorld()
	now := time.Now()
	w.gifts.WithClock(func() time.Time { return now })
	ann, bob, cat := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	sent, err := w.gifts.Send(ctx, ann, cat, giftDraft("freenet_ring"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.gifts.Upgrade(ctx, cat, sent.ID); err != nil {
		t.Fatal(err)
	}

	hours, _ := user.NewAuctionHours(1)
	if err = w.gifts.StartAuction(ctx, cat, sent.ID, 10, hours); err != nil {
		t.Fatal(err)
	}
	if _, err = w.gifts.SetSale(ctx, cat, sent.ID, 50); !errors.Is(err, user.ErrGiftOnSale) {
		t.Fatalf("an auctioned gift put on sale: %v", err)
	}
	if _, err = w.gifts.Bid(ctx, cat, sent.ID, 10); !errors.Is(err, user.ErrOwnGift) {
		t.Fatalf("the seller bids: %v", err)
	}

	annVotes, bobVotes, catVotes := w.votes(t, ann), w.votes(t, bob), w.votes(t, cat)

	if _, err = w.gifts.Bid(ctx, ann, sent.ID, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = w.gifts.Bid(ctx, bob, sent.ID, 10); !errors.Is(err, user.ErrBidTooLow) {
		t.Fatalf("a bid that does not beat the top one: %v", err)
	}
	if _, err = w.gifts.Bid(ctx, bob, sent.ID, 11); err != nil {
		t.Fatal(err)
	}
	if w.votes(t, ann) != annVotes || w.votes(t, bob) != bobVotes-11 {
		t.Fatalf("the beaten bid is not back: ann %d of %d", w.votes(t, ann), annVotes)
	}
	if err = w.gifts.CancelAuction(ctx, cat, sent.ID); !errors.Is(err, user.ErrAuctionHasBids) {
		t.Fatalf("an auction with bids cancelled: %v", err)
	}

	// Raising one's own bid holds only what it adds.
	if _, err = w.gifts.Bid(ctx, bob, sent.ID, 20); err != nil {
		t.Fatal(err)
	}
	if w.votes(t, bob) != bobVotes-20 {
		t.Fatalf("bob has %d after raising to 20 from %d", w.votes(t, bob), bobVotes)
	}

	if n, err := w.gifts.SettleDue(ctx); err != nil || n != 0 {
		t.Fatalf("a running auction settled: %d, %v", n, err)
	}

	now = now.Add(2 * time.Hour)
	if n, err := w.gifts.SettleDue(ctx); err != nil || n != 1 {
		t.Fatalf("settled %d, %v", n, err)
	}
	// 5% of 20 is the fee.
	if w.votes(t, cat) != catVotes+19 {
		t.Fatalf("the seller got %d", w.votes(t, cat)-catVotes)
	}

	got, err := w.gifts.Get(ctx, ann, sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner == nil || got.Owner.ID != bob || got.Auction != nil || got.History[0].Kind != user.GiftEventAuction {
		t.Fatalf("after the auction: %+v", got)
	}
}

func TestGiftService_MintAuction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newGiftWorld()
	now := time.Now()
	w.gifts.WithClock(func() time.Time { return now })
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	traits, _ := user.NewGiftTraits("onyx", "gold", "crown")
	hours, _ := user.NewAuctionHours(24)
	supply, _ := user.NewGiftSupply(1)

	if _, err := w.gifts.MintAuction(ctx, ann, "freenet_crown", traits, 100, hours); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("not an administrator mints: %v", err)
	}
	if err := w.gifts.SetSupply(ctx, admin, "freenet_crown", supply); err != nil {
		t.Fatal(err)
	}

	id, err := w.gifts.MintAuction(ctx, admin, "freenet_crown", traits, 100, hours)
	if err != nil {
		t.Fatal(err)
	}
	// The edition of one is gone.
	if _, err = w.gifts.Send(ctx, ann, ann, giftDraft("freenet_crown")); !errors.Is(err, user.ErrGiftSoldOut) {
		t.Fatalf("a gift sent past its edition: %v", err)
	}

	page, err := w.gifts.Auctions(ctx, ann, "", 10)
	if err != nil || len(page.Items) != 1 || !page.Items[0].Site || page.Items[0].Auction.MinBid != 100 {
		t.Fatalf("auctions = %+v, %v", page, err)
	}

	treasury, _ := w.users.Treasury(ctx)
	if _, err = w.gifts.Bid(ctx, ann, id, 150); err != nil {
		t.Fatal(err)
	}

	now = now.Add(25 * time.Hour)
	if _, err = w.gifts.SettleDue(ctx); err != nil {
		t.Fatal(err)
	}

	after, _ := w.users.Treasury(ctx)
	if after.Votes != treasury.Votes+150 {
		t.Fatalf("the treasury got %d", after.Votes-treasury.Votes)
	}

	got, err := w.gifts.Get(ctx, ann, id)
	if err != nil || got.Owner == nil || got.Owner.ID != ann || got.Gift.Number != 1 || got.Gift.Traits != traits {
		t.Fatalf("the edition of the site: %+v, %v", got, err)
	}
}

func giftDraft(kind user.GiftKind) service.GiftDraft {
	return service.GiftDraft{Kind: kind, Caption: "Привет"}
}
