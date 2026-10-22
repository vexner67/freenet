package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCollectibles(t *testing.T) {
	repo, pool := newRepo(t)
	gifts := postgres.NewGiftRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("collectible") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	ann, bob := newUser(), newUser()

	// The stock: a limited edition sells out, minting counts on.
	const kind user.GiftKind = "prime_rocket"

	before, err := gifts.Stocks(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sold := before[kind].Sold
	if err = gifts.SetSupply(ctx, kind, user.GiftSupply(sold+1)); err != nil {
		t.Fatal(err)
	}
	if err = gifts.Sell(ctx, kind); err != nil {
		t.Fatalf("Sell: %v", err)
	}
	if err = gifts.Sell(ctx, kind); !errors.Is(err, user.ErrGiftSoldOut) {
		t.Fatalf("Sell past the edition: %v", err)
	}
	if err = gifts.SetSupply(ctx, kind, 0); err != nil {
		t.Fatal(err)
	}

	first, err := gifts.Mint(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gifts.Mint(ctx, kind)
	if err != nil || second != first+1 {
		t.Fatalf("Mint = %d after %d, %v", second, first, err)
	}

	after, err := gifts.Stocks(ctx)
	if err != nil || after[kind].Sold != sold+1 || after[kind].Minted != second || after[kind].Supply != 0 {
		t.Fatalf("Stocks = %+v, %v", after[kind], err)
	}

	// A collectible of ann's, put on sale, then passed to bob.
	g, err := ann.GiftFrom(bob.ID(), user.Ties{}, kind, "Привет", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.Save(ctx, g); err != nil {
		t.Fatal(err)
	}

	traits := user.GiftTraits{Model: "amethyst", Backdrop: "black", Symbol: "rocket"}
	upgraded, err := g.Upgraded(ann.ID(), first, traits)
	if err != nil {
		t.Fatal(err)
	}
	onSale, err := upgraded.OnSale(ann.ID(), 70)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.Update(ctx, onSale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	locked, err := gifts.Lock(ctx, g.ID())
	if err != nil || locked.Number() != first || locked.Traits() != traits || locked.SalePrice() != 70 {
		t.Fatalf("Lock = %+v, %v", locked, err)
	}

	market, err := gifts.Market(ctx, user.MarketFilter{Kind: kind, Model: "amethyst", Sort: user.MarketCheapest}, nil, 10)
	if err != nil || len(market) != 1 || market[0].ID() != g.ID() {
		t.Fatalf("Market = %v, %v", market, err)
	}

	key := user.MarketFilter{Sort: user.MarketCheapest}.Of(market[0])
	if rest, err := gifts.Market(ctx, user.MarketFilter{Kind: kind, Model: "amethyst", Sort: user.MarketCheapest}, &key, 10); err != nil || len(rest) != 0 {
		t.Fatalf("Market after the last = %v, %v", rest, err)
	}

	if err = gifts.Update(ctx, onSale.PassedTo(bob.ID())); err != nil {
		t.Fatal(err)
	}
	if got, err := gifts.Get(ctx, g.ID()); err != nil || got.OwnerID() != bob.ID() || got.SenderID() != ann.ID() || got.IsOnSale() || got.Caption() != "" {
		t.Fatalf("Get after passing = %+v, %v", got, err)
	}
	if market, err = gifts.Market(ctx, user.MarketFilter{Kind: kind, Model: "amethyst", Sort: user.MarketCheapest}, nil, 10); err != nil || len(market) != 0 {
		t.Fatalf("Market off sale = %v, %v", market, err)
	}

	missing, err := user.MintedGift(kind, 0, traits)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.Update(ctx, missing); !errors.Is(err, user.ErrGiftNotFound) {
		t.Fatalf("Update of nothing: %v", err)
	}

	// History, newest first.
	for _, e := range []struct {
		kind     user.GiftEventKind
		from, to uuid.UUID
		price    int64
	}{
		{user.GiftEventUpgrade, uuid.Nil, ann.ID(), 0},
		{user.GiftEventSale, ann.ID(), bob.ID(), 70},
	} {
		ev, err := user.NewGiftEvent(g.ID(), e.kind, e.from, e.to, e.price)
		if err != nil {
			t.Fatal(err)
		}
		if err = gifts.SaveEvent(ctx, ev); err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
	}

	history, err := gifts.History(ctx, g.ID(), 10)
	if err != nil || len(history) != 2 || history[0].Kind != user.GiftEventSale || history[0].Price != 70 ||
		history[0].FromID != ann.ID() || history[1].Kind != user.GiftEventUpgrade || history[1].FromID != uuid.Nil {
		t.Fatalf("History = %+v, %v", history, err)
	}

	// An auction of bob's: bid on, due when it ends, deleted.
	now := time.Now().UTC().Truncate(time.Microsecond)

	a, err := user.NewGiftAuction(g.ID(), bob.ID(), 10, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.SaveAuction(ctx, a); err != nil {
		t.Fatalf("SaveAuction: %v", err)
	}

	bid, err := a.Bid(ann.ID(), 12, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.SaveAuction(ctx, bid); err != nil {
		t.Fatal(err)
	}

	got, err := gifts.LockAuction(ctx, g.ID())
	if err != nil || got.TopBid() != 12 || got.BidderID() != ann.ID() || got.Bids() != 1 || !got.EndsAt().Equal(a.EndsAt()) {
		t.Fatalf("LockAuction = %+v, %v", got, err)
	}

	auctioned, err := gifts.AuctionedIDs(ctx, []uuid.UUID{g.ID(), uuid.New()})
	if err != nil || !auctioned[g.ID()] || len(auctioned) != 1 {
		t.Fatalf("AuctionedIDs = %v, %v", auctioned, err)
	}

	running, err := gifts.Auctions(ctx, now, nil, 1000)
	if err != nil || !hasAuction(running, g.ID()) {
		t.Fatalf("Auctions = %v, %v", running, err)
	}
	if running, err = gifts.Auctions(ctx, now, &service.AuctionKey{EndsAt: a.EndsAt(), GiftID: g.ID()}, 1000); err != nil || hasAuction(running, g.ID()) {
		t.Fatalf("Auctions after it = %v, %v", running, err)
	}

	due, err := gifts.DueAuctions(ctx, now, 1000)
	if err != nil || hasID(due, g.ID()) {
		t.Fatalf("DueAuctions now = %v, %v", due, err)
	}
	if due, err = gifts.DueAuctions(ctx, a.EndsAt(), 1000); err != nil || !hasID(due, g.ID()) {
		t.Fatalf("DueAuctions at the end = %v, %v", due, err)
	}

	if err = gifts.DeleteAuction(ctx, g.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = gifts.Auction(ctx, g.ID()); !errors.Is(err, user.ErrAuctionNotFound) {
		t.Fatalf("Auction after deleting: %v", err)
	}

	// A gift the site holds has no recipient.
	minted, err := user.MintedGift(kind, second, traits)
	if err != nil {
		t.Fatal(err)
	}
	if err = gifts.Save(ctx, minted); err != nil {
		t.Fatalf("Save of a minted gift: %v", err)
	}
	if got, err := gifts.Get(ctx, minted.ID()); err != nil || !got.IsHeldBySite() || got.Number() != second {
		t.Fatalf("Get of a minted gift = %+v, %v", got, err)
	}
}

func hasAuction(list []*user.GiftAuction, id uuid.UUID) bool {
	for _, a := range list {
		if a.GiftID() == id {
			return true
		}
	}

	return false
}

func hasID(list []uuid.UUID, id uuid.UUID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}

	return false
}
