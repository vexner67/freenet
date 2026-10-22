package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// GiftStockRepository keeps how many of each gift there may be, have been
// sent and have been made collectible.
type GiftStockRepository interface {
	// Stocks are the stocks of every gift sent or limited so far.
	Stocks(ctx context.Context) (map[user.GiftKind]user.GiftStock, error)
	SetSupply(ctx context.Context, kind user.GiftKind, supply user.GiftSupply) error
	// Sell counts one more of kind sent; user.ErrGiftSoldOut when its
	// edition is all sent.
	Sell(ctx context.Context, kind user.GiftKind) error
	// Mint is the next number of a collectible of kind.
	Mint(ctx context.Context, kind user.GiftKind) (int, error)
}

// CollectibleRepository keeps what makes a gift collectible and its
// history.
type CollectibleRepository interface {
	// Lock reads a gift and holds it until the transaction ends;
	// user.ErrGiftNotFound.
	Lock(ctx context.Context, id uuid.UUID) (*user.Gift, error)
	// Update writes the owner, the sender, the words, the number, the
	// traits and the price of a gift.
	Update(ctx context.Context, g *user.Gift) error
	// Market returns up to limit gifts on sale matching f, past after
	// (nil: from the first), of pages neither deleted nor banned.
	Market(ctx context.Context, f user.MarketFilter, after *user.MarketKey, limit int) ([]*user.Gift, error)
	SaveEvent(ctx context.Context, e user.GiftEvent) error
	// History is up to limit lines of the history of a gift, newest first.
	History(ctx context.Context, giftID uuid.UUID, limit int) ([]user.GiftEvent, error)
}

// AuctionKey is where a page of the auctions goes on from: the end of the
// last auction and its gift.
type AuctionKey struct {
	EndsAt time.Time
	GiftID uuid.UUID
}

// AuctionRepository keeps the auctions of collectibles, one per gift.
type AuctionRepository interface {
	// SaveAuction writes an auction, new or bid on.
	SaveAuction(ctx context.Context, a *user.GiftAuction) error
	// Auction returns user.ErrAuctionNotFound.
	Auction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error)
	// LockAuction is Auction held until the transaction ends.
	LockAuction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error)
	// DeleteAuction is idempotent.
	DeleteAuction(ctx context.Context, giftID uuid.UUID) error
	// Auctions returns up to limit auctions running at now, ending
	// soonest first, past after (nil: from the first).
	Auctions(ctx context.Context, now time.Time, after *AuctionKey, limit int) ([]*user.GiftAuction, error)
	// DueAuctions are up to limit gifts whose auctions ended by now.
	DueAuctions(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	// AuctionedIDs tells which of ids are at an auction.
	AuctionedIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error)
}

const (
	// MaxGiftHistory is how many lines of its history a collectible shows.
	MaxGiftHistory = 50
	// MaxDueAuctions is how many ended auctions SettleDue settles a round.
	MaxDueAuctions = 100
)

// ErrCursor is a cursor of the market or the auctions that is not one.
var ErrCursor = errors.New("invalid cursor")

// AuctionView is an auction as it is shown: the seller (nil: the site or
// a deleted page), the top bidder and the least bid taken now. Mine tells
// the viewer that the top bid is theirs.
type AuctionView struct {
	Start  int64
	TopBid int64
	MinBid int64
	Bids   int
	EndsAt time.Time
	Site   bool
	Seller *user.PublicProfile
	Bidder *user.PublicProfile
	Mine   bool
}

// HistoryLine is a line of the history of a collectible with the people
// in it (nil: the site, or a page deleted or erased).
type HistoryLine struct {
	Kind  user.GiftEventKind
	At    time.Time
	Price int64
	From  *user.PublicProfile
	To    *user.PublicProfile
}

// Collectible is a collectible as a viewer sees it: the gift, its owner
// (nil: held by the site, or a deleted page), its auction and its history.
type Collectible struct {
	Gift    SentGift
	Owner   *user.PublicProfile
	Site    bool
	Mine    bool
	Auction *AuctionView
	History []HistoryLine
}

// CollectiblePage is one screen of the market or the auctions; NextCursor
// is "" on the last.
type CollectiblePage struct {
	Items      []Collectible
	NextCursor string
}

// WithRoll makes the traits of collectibles roll by roll, a random number
// in [0, n): tests pick them.
func (s *GiftService) WithRoll(roll func(n int64) (int64, error)) *GiftService {
	s.roll = roll

	return s
}

// WithClock makes the auctions run by now: tests end them.
func (s *GiftService) WithClock(now func() time.Time) *GiftService {
	s.now = now

	return s
}

// Upgrade makes the gift of ownerID a collectible for user.UpgradePrice
// votes: the next number of its kind and traits rolled by their rarity.
// user.ErrGiftNotFound, user.ErrNotGiftOwner, user.ErrGiftCollectible,
// user.ErrNotEnoughVotes.
func (s *GiftService) Upgrade(ctx context.Context, ownerID, giftID uuid.UUID) (SentGift, error) {
	var upgraded *user.Gift

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		g, err := s.collectibles.Lock(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.collectibles.Lock", err)
		}

		// Checked before the votes are taken: Upgraded checks it again.
		if err = g.CheckOwner(ownerID); err != nil {
			return err
		}

		if g.IsCollectible() {
			return user.ErrGiftCollectible
		}

		if _, err = s.wallets.Spend(ctx, ownerID, user.UpgradePrice, user.Entry(user.VoteGiftUpgrade, uuid.Nil, string(g.Kind()))); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		number, err := s.stock.Mint(ctx, g.Kind())
		if err != nil {
			return errs.Wrap("s.stock.Mint", err)
		}

		traits, err := user.RollGiftTraits(s.roll)
		if err != nil {
			return errs.Wrap("user.RollGiftTraits", err)
		}

		if upgraded, err = g.Upgraded(ownerID, number, traits); err != nil {
			return errs.Wrap("g.Upgraded", err)
		}

		if err = s.collectibles.Update(ctx, upgraded); err != nil {
			return errs.Wrap("s.collectibles.Update", err)
		}

		return errs.Wrap("s.event", s.event(ctx, upgraded, user.GiftEventUpgrade, uuid.Nil, ownerID, user.UpgradePrice))
	})
	if err != nil {
		return SentGift{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return s.one(ctx, ownerID, upgraded)
}

// Transfer gives the collectible of ownerID to toID for
// user.GiftTransferPrice votes; toID hears of it as of a gift from
// ownerID. As Send: user.ErrDeactivated, user.ErrGiftRefused; and
// user.ErrOwnGift, user.ErrNotCollectible, user.ErrGiftOnSale.
func (s *GiftService) Transfer(ctx context.Context, ownerID, toID, giftID uuid.UUID) error {
	if ownerID == toID {
		return user.ErrOwnGift
	}

	if err := s.checkTakes(ctx, ownerID, toID); err != nil {
		return errs.Wrap("s.checkTakes", err)
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		g, err := s.tradable(ctx, ownerID, giftID)
		if err != nil {
			return errs.Wrap("s.tradable", err)
		}

		if _, err = s.wallets.Spend(ctx, ownerID, user.GiftTransferPrice, user.Entry(user.VoteGiftTransfer, toID, noteOf(g))); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		return errs.Wrap("s.pass", s.pass(ctx, g, toID, user.GiftEventTransfer, 0))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// SetSale puts the collectible of ownerID up for price votes, or takes it
// off with 0; a gift at an auction is not put on sale
// (user.ErrGiftOnSale). user.ErrGiftSalePrice.
func (s *GiftService) SetSale(ctx context.Context, ownerID, giftID uuid.UUID, price int64) (SentGift, error) {
	var priced *user.Gift

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		g, err := s.collectibles.Lock(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.collectibles.Lock", err)
		}

		if err = s.checkNoAuction(ctx, giftID); err != nil {
			return errs.Wrap("s.checkNoAuction", err)
		}

		if priced, err = g.OnSale(ownerID, price); err != nil {
			return errs.Wrap("g.OnSale", err)
		}

		return errs.Wrap("s.collectibles.Update", s.collectibles.Update(ctx, priced))
	})
	if err != nil {
		return SentGift{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return s.one(ctx, ownerID, priced)
}

// Buy buys a collectible on sale for buyerID at price, the price they
// were shown: the votes go to its owner, who pays the treasury a fee of
// them (User.GiftFeeAt). It returns what the buyer has left.
// user.ErrGiftNotOnSale, user.ErrGiftPriceChanged, user.ErrOwnGift,
// user.ErrGiftRefused, user.ErrNotEnoughVotes.
func (s *GiftService) Buy(ctx context.Context, buyerID, giftID uuid.UUID, price int64) (int64, error) {
	g, err := s.giftRepo.Get(ctx, giftID)
	if err != nil {
		return 0, errs.Wrap("s.giftRepo.Get", err)
	}

	if err = g.CheckBuy(buyerID, price); err != nil {
		return 0, err
	}

	sellerID := g.OwnerID()

	seller, err := s.userRepo.GetByID(ctx, sellerID)
	if err != nil {
		return 0, errs.Wrap("s.userRepo.GetByID", err)
	}

	// The market does not show what deleted and banned pages sell.
	if seller.IsDeactivated() || seller.IsBannedAt(s.now()) {
		return 0, user.ErrGiftNotOnSale
	}

	if err = s.checkTakes(ctx, sellerID, buyerID); err != nil {
		return 0, errs.Wrap("s.checkTakes", err)
	}

	var balance int64

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		g, err := s.collectibles.Lock(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.collectibles.Lock", err)
		}

		// What it is now: sold meanwhile, it is someone else's.
		if err = g.CheckBuy(buyerID, price); err != nil {
			return err
		}

		if g.OwnerID() != sellerID {
			return user.ErrGiftPriceChanged
		}

		note := noteOf(g)

		if balance, err = s.wallets.Move(ctx, buyerID, sellerID, price, user.Entry(user.VoteGiftSale, uuid.Nil, note)); err != nil {
			return errs.Wrap("s.wallets.Move", err)
		}

		if err = s.payFee(ctx, seller, buyerID, price, note); err != nil {
			return errs.Wrap("s.payFee", err)
		}

		return errs.Wrap("s.pass", s.pass(ctx, g, buyerID, user.GiftEventSale, price))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return balance, nil
}

// StartAuction puts the collectible of ownerID up for the highest bid,
// from start, for hours. user.ErrNotCollectible, user.ErrGiftOnSale.
func (s *GiftService) StartAuction(ctx context.Context, ownerID, giftID uuid.UUID, start int64, hours user.AuctionHours) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.tradable(ctx, ownerID, giftID); err != nil {
			return errs.Wrap("s.tradable", err)
		}

		a, err := user.NewGiftAuction(giftID, ownerID, start, hours, s.now())
		if err != nil {
			return errs.Wrap("user.NewGiftAuction", err)
		}

		return errs.Wrap("s.auctions.SaveAuction", s.auctions.SaveAuction(ctx, a))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// MintAuction makes a collectible of kind with traits that the site holds
// and auctions it from start for hours: a rare edition, counted in the
// edition of kind. It returns the ID of the gift; user.ErrNotAdmin,
// user.ErrGiftSoldOut.
func (s *GiftService) MintAuction(ctx context.Context, adminID uuid.UUID, kind user.GiftKind, traits user.GiftTraits, start int64, hours user.AuctionHours) (uuid.UUID, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return uuid.Nil, errs.Wrap("checkAdmin", err)
	}

	var id uuid.UUID

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.stock.Sell(ctx, kind); err != nil {
			return errs.Wrap("s.stock.Sell", err)
		}

		number, err := s.stock.Mint(ctx, kind)
		if err != nil {
			return errs.Wrap("s.stock.Mint", err)
		}

		g, err := user.MintedGift(kind, number, traits)
		if err != nil {
			return errs.Wrap("user.MintedGift", err)
		}

		a, err := user.NewGiftAuction(g.ID(), uuid.Nil, start, hours, s.now())
		if err != nil {
			return errs.Wrap("user.NewGiftAuction", err)
		}

		if err = s.giftRepo.Save(ctx, g); err != nil {
			return errs.Wrap("s.giftRepo.Save", err)
		}

		if err = s.event(ctx, g, user.GiftEventMint, uuid.Nil, uuid.Nil, 0); err != nil {
			return errs.Wrap("s.event", err)
		}

		id = g.ID()

		return errs.Wrap("s.auctions.SaveAuction", s.auctions.SaveAuction(ctx, a))
	})
	if err != nil {
		return uuid.Nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return id, nil
}

// CancelAuction takes back an auction nobody has bid on: its seller's, or
// one of the site by an administrator — the gift of the site goes with
// it. user.ErrAuctionNotFound, user.ErrNotGiftOwner,
// user.ErrAuctionHasBids.
func (s *GiftService) CancelAuction(ctx context.Context, userID, giftID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.auctions.LockAuction(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.auctions.LockAuction", err)
		}

		if err = s.checkSeller(ctx, userID, a); err != nil {
			return errs.Wrap("s.checkSeller", err)
		}

		if err = a.CheckCancel(); err != nil {
			return err
		}

		if err = s.auctions.DeleteAuction(ctx, giftID); err != nil {
			return errs.Wrap("s.auctions.DeleteAuction", err)
		}

		if a.SellerID() != uuid.Nil {
			return nil
		}

		return errs.Wrap("s.giftRepo.Delete", s.giftRepo.Delete(ctx, giftID))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

func (s *GiftService) checkSeller(ctx context.Context, userID uuid.UUID, a *user.GiftAuction) error {
	if a.SellerID() == uuid.Nil {
		return errs.Wrap("checkAdmin", checkAdmin(ctx, s.userRepo, s.admins, userID))
	}

	if a.SellerID() != userID {
		return user.ErrNotGiftOwner
	}

	return nil
}

// Bid bids amount of bidderID at the auction of a gift: their votes are
// held, and those of the bid it beats go back to its bidder. It returns
// what the bidder has left. user.ErrAuctionNotFound,
// user.ErrAuctionEnded, user.ErrBidTooLow, user.ErrOwnGift,
// user.ErrGiftRefused, user.ErrNotEnoughVotes.
func (s *GiftService) Bid(ctx context.Context, bidderID, giftID uuid.UUID, amount int64) (int64, error) {
	a, err := s.auctions.Auction(ctx, giftID)
	if err != nil {
		return 0, errs.Wrap("s.auctions.Auction", err)
	}

	if a.SellerID() != uuid.Nil && a.SellerID() != bidderID {
		if err = s.checkTakes(ctx, a.SellerID(), bidderID); err != nil {
			return 0, errs.Wrap("s.checkTakes", err)
		}
	}

	var balance int64

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.auctions.LockAuction(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.auctions.LockAuction", err)
		}

		bid, err := a.Bid(bidderID, amount, s.now())
		if err != nil {
			return errs.Wrap("a.Bid", err)
		}

		g, err := s.giftRepo.Get(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.giftRepo.Get", err)
		}

		if balance, err = s.hold(ctx, a, bidderID, amount, noteOf(g)); err != nil {
			return errs.Wrap("s.hold", err)
		}

		return errs.Wrap("s.auctions.SaveAuction", s.auctions.SaveAuction(ctx, bid))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return balance, nil
}

// hold holds the votes of a new top bid and gives those of the beaten one
// back. Raising one's own bid holds only what it adds. The new votes are
// taken first: where nothing rolls back (inmem), a bid short of votes
// changes nothing.
func (s *GiftService) hold(ctx context.Context, a *user.GiftAuction, bidderID uuid.UUID, amount int64, note string) (int64, error) {
	if a.HasWinner() && a.BidderID() == bidderID {
		balance, err := s.wallets.Hold(ctx, bidderID, amount-a.TopBid(), user.Entry(user.VoteGiftBid, uuid.Nil, note))
		return balance, errs.Wrap("s.wallets.Hold", err)
	}

	balance, err := s.wallets.Hold(ctx, bidderID, amount, user.Entry(user.VoteGiftBid, uuid.Nil, note))
	if err != nil {
		return 0, errs.Wrap("s.wallets.Hold", err)
	}

	if !a.HasWinner() {
		return balance, nil
	}

	_, err = s.wallets.Release(ctx, a.BidderID(), a.TopBid(), user.Entry(user.VoteGiftRefund, bidderID, note))
	if errors.Is(err, user.ErrNotFound) {
		// The beaten bidder's page is erased: their votes are gone with it.
		err = nil
	}

	return balance, errs.Wrap("s.wallets.Release", err)
}

// SettleDue settles the auctions that ended: the top bidder gets the gift
// and the seller the votes (less the fee), the treasury those of an
// auction of the site; a gift nobody bid on stays its seller's, and one of
// the site goes. It returns how many it settled; a failure of one does not
// keep back the rest.
func (s *GiftService) SettleDue(ctx context.Context) (int, error) {
	due, err := s.auctions.DueAuctions(ctx, s.now(), MaxDueAuctions)
	if err != nil {
		return 0, errs.Wrap("s.auctions.DueAuctions", err)
	}

	var (
		settled int
		failed  []error
	)

	for _, id := range due {
		if err := s.settle(ctx, id); err != nil {
			failed = append(failed, errs.Wrap("s.settle", err))
			continue
		}

		settled++
	}

	return settled, errors.Join(failed...)
}

func (s *GiftService) settle(ctx context.Context, giftID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.auctions.LockAuction(ctx, giftID)
		if errors.Is(err, user.ErrAuctionNotFound) {
			return nil // settled by another replica
		}

		if err != nil {
			return errs.Wrap("s.auctions.LockAuction", err)
		}

		if !a.Ended(s.now()) {
			return nil // a bid moved the end on
		}

		g, err := s.collectibles.Lock(ctx, giftID)
		if err != nil {
			return errs.Wrap("s.collectibles.Lock", err)
		}

		if err = s.auctions.DeleteAuction(ctx, giftID); err != nil {
			return errs.Wrap("s.auctions.DeleteAuction", err)
		}

		if !a.HasWinner() {
			if g.IsHeldBySite() {
				return errs.Wrap("s.giftRepo.Delete", s.giftRepo.Delete(ctx, giftID))
			}

			return nil
		}

		if err = s.payOut(ctx, a, noteOf(g)); err != nil {
			return errs.Wrap("s.payOut", err)
		}

		return errs.Wrap("s.pass", s.pass(ctx, g, a.BidderID(), user.GiftEventAuction, a.TopBid()))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// payOut pays the held top bid of an ended auction to its seller, who pays
// the fee of it, or into the treasury for an auction of the site.
func (s *GiftService) payOut(ctx context.Context, a *user.GiftAuction, note string) error {
	if a.SellerID() == uuid.Nil {
		return errs.Wrap("s.wallets.Deposit", s.wallets.Deposit(ctx, a.TopBid()))
	}

	seller, err := s.userRepo.GetByID(ctx, a.SellerID())
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if _, err = s.wallets.Release(ctx, a.SellerID(), a.TopBid(), user.Entry(user.VoteGiftSale, a.BidderID(), note)); err != nil {
		return errs.Wrap("s.wallets.Release", err)
	}

	return errs.Wrap("s.payFee", s.payFee(ctx, seller, a.BidderID(), a.TopBid(), note))
}

// payFee takes the fee of a sale for price out of what seller was paid.
func (s *GiftService) payFee(ctx context.Context, seller *user.User, buyerID uuid.UUID, price int64, note string) error {
	fee := seller.GiftFeeAt(price, s.now())
	if fee == 0 {
		return nil
	}

	_, err := s.wallets.Spend(ctx, seller.ID(), fee, user.Entry(user.VoteGiftFee, buyerID, note))

	return errs.Wrap("s.wallets.Spend", err)
}

// tradable is the gift of ownerID locked for a trade: their collectible,
// neither on sale nor at an auction.
func (s *GiftService) tradable(ctx context.Context, ownerID, giftID uuid.UUID) (*user.Gift, error) {
	g, err := s.collectibles.Lock(ctx, giftID)
	if err != nil {
		return nil, errs.Wrap("s.collectibles.Lock", err)
	}

	if err = g.CheckTrade(ownerID); err != nil {
		return nil, err
	}

	if err = s.checkNoAuction(ctx, giftID); err != nil {
		return nil, errs.Wrap("s.checkNoAuction", err)
	}

	return g, nil
}

func (s *GiftService) checkNoAuction(ctx context.Context, giftID uuid.UUID) error {
	_, err := s.auctions.Auction(ctx, giftID)
	if errors.Is(err, user.ErrAuctionNotFound) {
		return nil
	}

	if err != nil {
		return errs.Wrap("s.auctions.Auction", err)
	}

	return user.ErrGiftOnSale
}

// checkOff: a gift on sale or at an auction is not deleted.
func (s *GiftService) checkOff(ctx context.Context, g *user.Gift) error {
	if g.IsOnSale() {
		return user.ErrGiftOnSale
	}

	if !g.IsCollectible() {
		return nil
	}

	return errs.Wrap("s.checkNoAuction", s.checkNoAuction(ctx, g.ID()))
}

// checkTakes tells whether toID takes a gift from fromID, as Send.
func (s *GiftService) checkTakes(ctx context.Context, fromID, toID uuid.UUID) error {
	recipient, ties, err := s.permissions.ties(ctx, fromID, toID)
	if err != nil {
		return errs.Wrap("s.permissions.ties", err)
	}

	return recipient.CheckGiftFrom(fromID, ties)
}

// pass hands the collectible g to toID and writes it down; the
// notification of the gift goes from its old owner to the new one.
func (s *GiftService) pass(ctx context.Context, g *user.Gift, toID uuid.UUID, kind user.GiftEventKind, price int64) error {
	passed := g.PassedTo(toID)

	if err := s.collectibles.Update(ctx, passed); err != nil {
		return errs.Wrap("s.collectibles.Update", err)
	}

	if err := s.event(ctx, g, kind, g.OwnerID(), toID, price); err != nil {
		return errs.Wrap("s.event", err)
	}

	// A gift is one notification: the old owner's goes first.
	if g.Notifies() {
		if err := s.notifier.GiftDeleted(ctx, g.ID(), g.SenderID(), g.RecipientID()); err != nil {
			return errs.Wrap("s.notifier.GiftDeleted", err)
		}
	}

	if !passed.Notifies() {
		return nil
	}

	return errs.Wrap("s.notifier.GiftSent", s.notifier.GiftSent(ctx, passed.ID(), passed.SenderID(), passed.RecipientID()))
}

func (s *GiftService) event(ctx context.Context, g *user.Gift, kind user.GiftEventKind, fromID, toID uuid.UUID, price int64) error {
	e, err := user.NewGiftEvent(g.ID(), kind, fromID, toID, price)
	if err != nil {
		return errs.Wrap("user.NewGiftEvent", err)
	}

	return errs.Wrap("s.collectibles.SaveEvent", s.collectibles.SaveEvent(ctx, e))
}

// noteOf names a collectible in statements: its kind and number.
func noteOf(g *user.Gift) string {
	return fmt.Sprintf("%s#%d", g.Kind(), g.Number())
}

// SetSupply limits the edition of kind to supply, 0 for no end; gifts
// sent already are not touched. user.ErrNotAdmin.
func (s *GiftService) SetSupply(ctx context.Context, adminID uuid.UUID, kind user.GiftKind, supply user.GiftSupply) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.stock.SetSupply", s.stock.SetSupply(ctx, kind, supply))
}

// Get is a collectible as viewerID sees it, with its auction and its
// history. One neither on sale nor at an auction shows only to those who
// see the gifts of its owner: user.ErrGiftsHidden. A gift that is not
// collectible is user.ErrGiftNotFound here.
func (s *GiftService) Get(ctx context.Context, viewerID, giftID uuid.UUID) (Collectible, error) {
	g, err := s.giftRepo.Get(ctx, giftID)
	if err != nil {
		return Collectible{}, errs.Wrap("s.giftRepo.Get", err)
	}

	if !g.IsCollectible() {
		return Collectible{}, user.ErrGiftNotFound
	}

	a, err := s.auctions.Auction(ctx, giftID)
	if errors.Is(err, user.ErrAuctionNotFound) {
		a, err = nil, nil
	}

	if err != nil {
		return Collectible{}, errs.Wrap("s.auctions.Auction", err)
	}

	if !g.IsOnSale() && a == nil && !g.IsHeldBySite() {
		owner, ties, err := s.permissions.ties(ctx, viewerID, g.OwnerID())
		if err != nil {
			return Collectible{}, errs.Wrap("s.permissions.ties", err)
		}

		if access := owner.AccessFor(viewerID, ties, s.now()); !access.Page || !access.Gifts {
			return Collectible{}, user.ErrGiftsHidden
		}
	}

	history, err := s.collectibles.History(ctx, giftID, MaxGiftHistory)
	if err != nil {
		return Collectible{}, errs.Wrap("s.collectibles.History", err)
	}

	auctions := map[uuid.UUID]*user.GiftAuction{}
	if a != nil {
		auctions[giftID] = a
	}

	items, err := s.collectiblesOf(ctx, viewerID, []*user.Gift{g}, auctions, history)
	if err != nil {
		return Collectible{}, errs.Wrap("s.collectiblesOf", err)
	}

	return items[0], nil
}

// Market pages the collectibles on sale matching f, as viewerID sees them.
func (s *GiftService) Market(ctx context.Context, viewerID uuid.UUID, f user.MarketFilter, cursor string, size int) (CollectiblePage, error) {
	after, err := parseMarketCursor(cursor)
	if err != nil {
		return CollectiblePage{}, err
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	found, err := s.collectibles.Market(ctx, f, after, size+1)
	if err != nil {
		return CollectiblePage{}, errs.Wrap("s.collectibles.Market", err)
	}

	var page CollectiblePage

	if len(found) > size {
		found = found[:size]
		key := f.Of(found[size-1])
		page.NextCursor = strconv.FormatInt(key.Value, 10) + "_" + key.ID.String()
	}

	if page.Items, err = s.collectiblesOf(ctx, viewerID, found, nil, nil); err != nil {
		return CollectiblePage{}, errs.Wrap("s.collectiblesOf", err)
	}

	return page, nil
}

// Auctions pages the running auctions, ending soonest first, as viewerID
// sees them.
func (s *GiftService) Auctions(ctx context.Context, viewerID uuid.UUID, cursor string, size int) (CollectiblePage, error) {
	after, err := parseAuctionCursor(cursor)
	if err != nil {
		return CollectiblePage{}, err
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	found, err := s.auctions.Auctions(ctx, s.now(), after, size+1)
	if err != nil {
		return CollectiblePage{}, errs.Wrap("s.auctions.Auctions", err)
	}

	var page CollectiblePage

	if len(found) > size {
		found = found[:size]
		last := found[size-1]
		page.NextCursor = strconv.FormatInt(last.EndsAt().UnixMicro(), 10) + "_" + last.GiftID().String()
	}

	byGift := make(map[uuid.UUID]*user.GiftAuction, len(found))
	gifts := make([]*user.Gift, 0, len(found))

	for _, a := range found {
		g, err := s.giftRepo.Get(ctx, a.GiftID())
		if errors.Is(err, user.ErrGiftNotFound) {
			continue
		}

		if err != nil {
			return CollectiblePage{}, errs.Wrap("s.giftRepo.Get", err)
		}

		byGift[a.GiftID()] = a
		gifts = append(gifts, g)
	}

	if page.Items, err = s.collectiblesOf(ctx, viewerID, gifts, byGift, nil); err != nil {
		return CollectiblePage{}, errs.Wrap("s.collectiblesOf", err)
	}

	return page, nil
}

// collectiblesOf shows gifts with their owners, auctions and history,
// reading the people of them in one go.
func (s *GiftService) collectiblesOf(ctx context.Context, viewerID uuid.UUID, gifts []*user.Gift, auctions map[uuid.UUID]*user.GiftAuction, history []user.GiftEvent) ([]Collectible, error) {
	shown, err := s.shown(ctx, viewerID, gifts)
	if err != nil {
		return nil, errs.Wrap("s.shown", err)
	}

	var ids []uuid.UUID

	for _, g := range gifts {
		ids = append(ids, g.OwnerID())
		if a := auctions[g.ID()]; a != nil {
			ids = append(ids, a.SellerID(), a.BidderID())
		}
	}

	for _, e := range history {
		ids = append(ids, e.FromID, e.ToID)
	}

	people, err := s.people(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.people", err)
	}

	items := make([]Collectible, len(gifts))

	for i, g := range gifts {
		items[i] = Collectible{
			Gift:  shown[i],
			Owner: people[g.OwnerID()],
			Site:  g.IsHeldBySite(),
			Mine:  viewerID != uuid.Nil && viewerID == g.OwnerID(),
		}

		if a := auctions[g.ID()]; a != nil {
			items[i].Auction = &AuctionView{
				Start:  a.Start(),
				TopBid: a.TopBid(),
				MinBid: a.MinBid(),
				Bids:   a.Bids(),
				EndsAt: a.EndsAt(),
				Site:   a.SellerID() == uuid.Nil,
				Seller: people[a.SellerID()],
				Bidder: people[a.BidderID()],
				Mine:   viewerID != uuid.Nil && a.HasWinner() && a.BidderID() == viewerID,
			}
		}
	}

	if len(items) == 1 {
		for _, e := range history {
			items[0].History = append(items[0].History, HistoryLine{
				Kind:  e.Kind,
				At:    e.At(),
				Price: e.Price,
				From:  people[e.FromID],
				To:    people[e.ToID],
			})
		}
	}

	return items, nil
}

// people are the listed profiles of ids, but for deleted pages and
// uuid.Nil.
func (s *GiftService) people(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*user.PublicProfile, error) {
	seen := make(map[uuid.UUID]bool, len(ids))
	wanted := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}

	people := make(map[uuid.UUID]*user.PublicProfile, len(wanted))
	if len(wanted) == 0 {
		return people, nil
	}

	found, err := s.userRepo.GetByIDs(ctx, wanted)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByIDs", err)
	}

	now := s.now()

	for _, u := range found {
		if !u.IsDeactivated() {
			profile := u.ListedProfile(now)
			people[u.ID()] = &profile
		}
	}

	return people, nil
}

// auctioned tells which of gifts are at an auction.
func (s *GiftService) auctioned(ctx context.Context, gifts []*user.Gift) (map[uuid.UUID]bool, error) {
	var ids []uuid.UUID

	for _, g := range gifts {
		if g.IsCollectible() {
			ids = append(ids, g.ID())
		}
	}

	if len(ids) == 0 {
		return map[uuid.UUID]bool{}, nil
	}

	found, err := s.auctions.AuctionedIDs(ctx, ids)

	return found, errs.Wrap("s.auctions.AuctionedIDs", err)
}

// one is a single gift as viewerID sees it.
func (s *GiftService) one(ctx context.Context, viewerID uuid.UUID, g *user.Gift) (SentGift, error) {
	shown, err := s.shown(ctx, viewerID, []*user.Gift{g})
	if err != nil {
		return SentGift{}, errs.Wrap("s.shown", err)
	}

	return shown[0], nil
}

func parseMarketCursor(cursor string) (*user.MarketKey, error) {
	if cursor == "" {
		return nil, nil
	}

	value, id, ok := splitCursor(cursor)
	if !ok {
		return nil, ErrCursor
	}

	return &user.MarketKey{Value: value, ID: id}, nil
}

func parseAuctionCursor(cursor string) (*AuctionKey, error) {
	if cursor == "" {
		return nil, nil
	}

	micros, id, ok := splitCursor(cursor)
	if !ok {
		return nil, ErrCursor
	}

	return &AuctionKey{EndsAt: time.UnixMicro(micros).UTC(), GiftID: id}, nil
}

// splitCursor reads "<number>_<uuid>".
func splitCursor(cursor string) (int64, uuid.UUID, bool) {
	head, tail, ok := strings.Cut(cursor, "_")
	if !ok {
		return 0, uuid.Nil, false
	}

	value, err := strconv.ParseInt(head, 10, 64)
	if err != nil {
		return 0, uuid.Nil, false
	}

	id, err := uuid.Parse(tail)
	if err != nil {
		return 0, uuid.Nil, false
	}

	return value, id, true
}
