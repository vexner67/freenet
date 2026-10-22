package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Collectible gifts, as Telegram's: a gift its owner upgrades for votes
// gets a number of its kind and three traits rolled by rarity — the model
// (its metal and stones), the backdrop and the symbol of the pattern. A
// collectible changes hands: given away, sold for a price its owner sets
// or at an auction, where the votes of the bids are held until someone
// bids more. The site itself auctions rare editions it mints. Every sink
// goes to the treasury: the upgrade, the transfer and a fee of each sale.
const (
	// UpgradePrice is what turning a gift into a collectible costs.
	UpgradePrice = 50
	// GiftTransferPrice is what giving a collectible away costs.
	GiftTransferPrice = 10
	// GiftFeePermille is what the treasury takes of a sale, in tenths of
	// a percent, rounded up; PrimeGiftFeePermille — of a seller with
	// Freenet Prime.
	GiftFeePermille      = 50
	PrimeGiftFeePermille = 25
	// MaxGiftSalePrice is the dearest a collectible may be sold or bid
	// for.
	MaxGiftSalePrice = MaxAmount
	// MinAuctionHours and MaxAuctionHours bound how long an auction runs.
	MinAuctionHours = 1
	MaxAuctionHours = 7 * 24
	// AuctionStepPercent is how much a bid has to beat the last one by,
	// rounded up: at least one vote.
	AuctionStepPercent = 5
	// AuctionExtension: a bid this close to the end moves the end so far
	// on, so that nobody wins by bidding in the last second.
	AuctionExtension = 5 * time.Minute
	// MaxGiftSupply is the largest edition a gift may be limited to.
	MaxGiftSupply = 1_000_000_000
	// TraitWeights is what the weights of every list of traits add up to:
	// a weight is the chance of the trait in tenths of a percent.
	TraitWeights = 1000
)

var (
	ErrGiftCollectible = errors.New("the gift is collectible already")
	ErrNotCollectible  = errors.New("the gift is not collectible")
	// ErrNotGiftOwner: a gift is upgraded, given away and sold by its
	// owner.
	ErrNotGiftOwner = errors.New("only the owner of a gift can do this")
	// ErrGiftOnSale: a gift on sale or at an auction is not given away,
	// deleted or put up again until it is taken off.
	ErrGiftOnSale       = errors.New("the gift is on sale")
	ErrGiftNotOnSale    = errors.New("the gift is not on sale")
	ErrGiftPriceChanged = errors.New("the price of the gift changed")
	ErrGiftSalePrice    = errors.New("the price of a gift must be at least 1 vote")
	// ErrOwnGift: one does not buy, bid on or give oneself what is one's
	// own.
	ErrOwnGift           = errors.New("this gift is yours")
	ErrGiftSoldOut       = errors.New("this gift is sold out")
	ErrGiftSupplyInvalid = errors.New("the edition of a gift must be from 0 to 1000000000")
	ErrUnknownTrait      = errors.New("unknown trait of a gift")
	ErrMarketSort        = errors.New("unknown order of the market")
	ErrAuctionHours      = errors.New("an auction runs from 1 to 168 hours")
	ErrAuctionNotFound   = errors.New("auction not found")
	ErrAuctionEnded      = errors.New("the auction is over")
	ErrBidTooLow         = errors.New("the bid is too low")
	ErrAuctionHasBids    = errors.New("an auction with bids cannot be cancelled")
)

// GiftTrait is a model, a backdrop or a symbol a collectible may roll,
// and how likely it is: Weight of TraitWeights.
type GiftTrait struct {
	Name   string
	Weight int
}

// The traits, commonest first. The site draws them: the models are
// pictures of scripts/giftgen, the backdrops and symbols its own.
var (
	giftModels = []GiftTrait{
		{"classic", 400}, {"silver", 250}, {"rose", 170}, {"onyx", 100}, {"emerald", 60}, {"amethyst", 20},
	}
	giftBackdrops = []GiftTrait{
		{"sky", 150}, {"mint", 140}, {"coral", 130}, {"sand", 120}, {"lavender", 110}, {"steel", 100},
		{"cherry", 80}, {"ocean", 60}, {"forest", 50}, {"night", 30}, {"gold", 20}, {"black", 10},
	}
	giftSymbols = []GiftTrait{
		{"f", 150}, {"star", 140}, {"heart", 130}, {"flower", 110}, {"note", 100}, {"moon", 90},
		{"bolt", 80}, {"clover", 70}, {"snowflake", 50}, {"crown", 40}, {"diamond", 25}, {"rocket", 15},
	}
)

// GiftModels, GiftBackdrops and GiftSymbols are the traits there are.
func GiftModels() []GiftTrait    { return append([]GiftTrait(nil), giftModels...) }
func GiftBackdrops() []GiftTrait { return append([]GiftTrait(nil), giftBackdrops...) }
func GiftSymbols() []GiftTrait   { return append([]GiftTrait(nil), giftSymbols...) }

// GiftTraits are what a collectible rolled.
type GiftTraits struct {
	Model    string
	Backdrop string
	Symbol   string
}

// NewGiftTraits takes only traits there are: the administrators choose
// them for an edition of the site.
func NewGiftTraits(model, backdrop, symbol string) (GiftTraits, error) {
	if !hasTrait(giftModels, model) || !hasTrait(giftBackdrops, backdrop) || !hasTrait(giftSymbols, symbol) {
		return GiftTraits{}, ErrUnknownTrait
	}

	return GiftTraits{Model: model, Backdrop: backdrop, Symbol: symbol}, nil
}

// NewTraitFilter takes a trait of list to look for, "" for any.
func NewTraitFilter(list []GiftTrait, name string) (string, error) {
	if name != "" && !hasTrait(list, name) {
		return "", ErrUnknownTrait
	}

	return name, nil
}

func hasTrait(list []GiftTrait, name string) bool {
	for _, t := range list {
		if t.Name == name {
			return true
		}
	}

	return false
}

// RollGiftTraits picks the three traits by their weights; roll is a
// random number in [0, n).
func RollGiftTraits(roll func(n int64) (int64, error)) (GiftTraits, error) {
	var picked [3]string

	for i, list := range [][]GiftTrait{giftModels, giftBackdrops, giftSymbols} {
		r, err := roll(TraitWeights)
		if err != nil {
			return GiftTraits{}, err
		}

		picked[i] = pickTrait(list, r)
	}

	return GiftTraits{Model: picked[0], Backdrop: picked[1], Symbol: picked[2]}, nil
}

// pickTrait is the trait of list r falls on, r in [0, TraitWeights).
func pickTrait(list []GiftTrait, r int64) string {
	for _, t := range list {
		if r < int64(t.Weight) {
			return t.Name
		}

		r -= int64(t.Weight)
	}

	return list[len(list)-1].Name
}

func (g *Gift) OwnerID() uuid.UUID  { return g.recipientID }
func (g *Gift) IsCollectible() bool { return g.number > 0 }
func (g *Gift) Number() int         { return g.number }
func (g *Gift) Traits() GiftTraits  { return g.traits }
func (g *Gift) SalePrice() int64    { return g.salePrice }
func (g *Gift) IsOnSale() bool      { return g.salePrice > 0 }
func (g *Gift) IsHeldBySite() bool  { return g.recipientID == uuid.Nil }
func (g *Gift) CheckOwner(id uuid.UUID) error {
	if id == uuid.Nil || id != g.recipientID {
		return ErrNotGiftOwner
	}

	return nil
}

// Upgraded is the gift of ownerID as the collectible of number with
// traits; its sender and words stay.
func (g *Gift) Upgraded(ownerID uuid.UUID, number int, traits GiftTraits) (*Gift, error) {
	if err := g.CheckOwner(ownerID); err != nil {
		return nil, err
	}

	if g.IsCollectible() {
		return nil, ErrGiftCollectible
	}

	return g.RestoreCollectible(number, traits, 0), nil
}

// CheckTrade tells whether ownerID may give away, sell or auction the
// gift: their own collectible, not on sale already.
func (g *Gift) CheckTrade(ownerID uuid.UUID) error {
	if err := g.CheckOwner(ownerID); err != nil {
		return err
	}

	if !g.IsCollectible() {
		return ErrNotCollectible
	}

	if g.IsOnSale() {
		return ErrGiftOnSale
	}

	return nil
}

// OnSale is the collectible of ownerID put up for price; 0 takes it off.
func (g *Gift) OnSale(ownerID uuid.UUID, price int64) (*Gift, error) {
	if err := g.CheckOwner(ownerID); err != nil {
		return nil, err
	}

	if !g.IsCollectible() {
		return nil, ErrNotCollectible
	}

	if price < 0 || price > MaxGiftSalePrice {
		return nil, ErrGiftSalePrice
	}

	return g.RestoreCollectible(g.number, g.traits, price), nil
}

// CheckBuy tells whether buyerID may buy the gift for price, what they
// were shown: a price changed meanwhile is not paid.
func (g *Gift) CheckBuy(buyerID uuid.UUID, price int64) error {
	switch {
	case !g.IsOnSale():
		return ErrGiftNotOnSale
	case g.recipientID == buyerID:
		return ErrOwnGift
	case g.salePrice != price:
		return ErrGiftPriceChanged
	default:
		return nil
	}
}

// PassedTo is the collectible as the gift of toID, come from its owner:
// the words and the anonymity of the first sender stay behind, and it is
// off sale.
func (g *Gift) PassedTo(toID uuid.UUID) *Gift {
	c := *g
	c.senderID, c.recipientID = g.recipientID, toID
	c.caption, c.anonymous, c.salePrice = "", false, 0

	return &c
}

// MintedGift is a collectible the site holds: an edition of the site to
// auction, with traits the administrators chose.
func MintedGift(kind GiftKind, number int, traits GiftTraits) (*Gift, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Gift{id: id, kind: kind, number: number, traits: traits}, nil
}

// GiftFeeAt is what the treasury takes of a sale of u's for price: the
// seller keeps the rest, rounded down.
func (u *User) GiftFeeAt(price int64, now time.Time) int64 {
	permille := int64(GiftFeePermille)
	if u.IsPrimeAt(now) {
		permille = PrimeGiftFeePermille
	}

	return price - price*(1000-permille)/1000
}

// GiftSupply is how many of a gift may ever be sent, 0 for no end: an
// edition the administrators limit.
type GiftSupply int

func NewGiftSupply(n int64) (GiftSupply, error) {
	if n < 0 || n > MaxGiftSupply {
		return 0, ErrGiftSupplyInvalid
	}

	return GiftSupply(n), nil
}

func (s GiftSupply) Int() int { return int(s) }

// GiftStock is how many of a gift there may be, have been sent and have
// been made collectible.
type GiftStock struct {
	Supply GiftSupply
	Sold   int
	Minted int
}

// Left is how many may still be sent, -1 for no end.
func (s GiftStock) Left() int {
	if s.Supply == 0 {
		return -1
	}

	return max(0, s.Supply.Int()-s.Sold)
}

// AuctionHours is how long an auction runs.
type AuctionHours int

func NewAuctionHours(n int) (AuctionHours, error) {
	if n < MinAuctionHours || n > MaxAuctionHours {
		return 0, ErrAuctionHours
	}

	return AuctionHours(n), nil
}

// GiftAuction is a collectible put up for the highest bid. The votes of
// the top bid are held from the bidder until someone bids more or the
// auction ends; then the seller — the treasury for an edition of the site
// — is paid and the bidder gets the gift.
type GiftAuction struct {
	giftID   uuid.UUID
	sellerID uuid.UUID
	start    int64
	endsAt   time.Time
	topBid   int64
	bidderID uuid.UUID
	bids     int
}

// NewGiftAuction puts giftID of sellerID (uuid.Nil: the site) up from
// start for hours.
func NewGiftAuction(giftID, sellerID uuid.UUID, start int64, hours AuctionHours, now time.Time) (*GiftAuction, error) {
	if start < 1 || start > MaxGiftSalePrice {
		return nil, ErrGiftSalePrice
	}

	return &GiftAuction{
		giftID:   giftID,
		sellerID: sellerID,
		start:    start,
		endsAt:   now.Add(time.Duration(hours) * time.Hour).UTC(),
	}, nil
}

// RestoreGiftAuction rebuilds an auction from storage.
func RestoreGiftAuction(giftID, sellerID uuid.UUID, start int64, endsAt time.Time, topBid int64, bidderID uuid.UUID, bids int) *GiftAuction {
	return &GiftAuction{
		giftID:   giftID,
		sellerID: sellerID,
		start:    start,
		endsAt:   endsAt,
		topBid:   topBid,
		bidderID: bidderID,
		bids:     bids,
	}
}

func (a *GiftAuction) GiftID() uuid.UUID   { return a.giftID }
func (a *GiftAuction) SellerID() uuid.UUID { return a.sellerID }
func (a *GiftAuction) Start() int64        { return a.start }
func (a *GiftAuction) EndsAt() time.Time   { return a.endsAt }
func (a *GiftAuction) TopBid() int64       { return a.topBid }
func (a *GiftAuction) BidderID() uuid.UUID { return a.bidderID }
func (a *GiftAuction) Bids() int           { return a.bids }

// Ended tells whether the auction takes no more bids.
func (a *GiftAuction) Ended(now time.Time) bool {
	return !now.Before(a.endsAt)
}

// HasWinner tells whether someone's bid stands: their votes are held.
func (a *GiftAuction) HasWinner() bool {
	return a.topBid > 0 && a.bidderID != uuid.Nil
}

// MinBid is the least bid taken now: the start, then the top bid and a
// step of it.
func (a *GiftAuction) MinBid() int64 {
	if !a.HasWinner() {
		return a.start
	}

	return a.topBid + max(1, (a.topBid*AuctionStepPercent+99)/100)
}

// Bid is the auction with amount of bidderID on top. A bid close to the
// end moves it on by AuctionExtension.
func (a *GiftAuction) Bid(bidderID uuid.UUID, amount int64, now time.Time) (*GiftAuction, error) {
	switch {
	case a.Ended(now):
		return nil, ErrAuctionEnded
	case bidderID == a.sellerID:
		return nil, ErrOwnGift
	case amount > MaxGiftSalePrice:
		return nil, ErrGiftSalePrice
	case amount < a.MinBid():
		return nil, ErrBidTooLow
	}

	b := *a
	b.topBid, b.bidderID, b.bids = amount, bidderID, a.bids+1

	if until := now.Add(AuctionExtension).UTC(); b.endsAt.Before(until) {
		b.endsAt = until
	}

	return &b, nil
}

// CheckCancel: an auction is taken back only while nobody has bid, for
// the votes held are someone's.
func (a *GiftAuction) CheckCancel() error {
	if a.HasWinner() {
		return ErrAuctionHasBids
	}

	return nil
}

// GiftEventKind is what happened to a collectible: a line of its history.
type GiftEventKind string

const (
	GiftEventUpgrade  GiftEventKind = "upgrade"
	GiftEventMint     GiftEventKind = "mint"
	GiftEventTransfer GiftEventKind = "transfer"
	GiftEventSale     GiftEventKind = "sale"
	GiftEventAuction  GiftEventKind = "auction"
)

// GiftEvent is a line of the history of a collectible: from whom to whom
// (uuid.Nil: the site, or an erased page) and for how much.
type GiftEvent struct {
	ID     uuid.UUID
	GiftID uuid.UUID
	Kind   GiftEventKind
	FromID uuid.UUID
	ToID   uuid.UUID
	Price  int64
}

func NewGiftEvent(giftID uuid.UUID, kind GiftEventKind, fromID, toID uuid.UUID, price int64) (GiftEvent, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return GiftEvent{}, err
	}

	return GiftEvent{ID: id, GiftID: giftID, Kind: kind, FromID: fromID, ToID: toID, Price: price}, nil
}

// At is the time of the event's UUIDv7.
func (e GiftEvent) At() time.Time {
	sec, nsec := e.ID.Time().UnixTime()

	return time.Unix(sec, nsec).UTC()
}

// MarketSort is the order of the market: cheapest first, dearest first or
// by number.
type MarketSort string

const (
	MarketCheapest MarketSort = "price"
	MarketDearest  MarketSort = "price_desc"
	MarketByNumber MarketSort = "number"
)

func NewMarketSort(s string) (MarketSort, error) {
	switch m := MarketSort(s); m {
	case "":
		return MarketCheapest, nil
	case MarketCheapest, MarketDearest, MarketByNumber:
		return m, nil
	default:
		return "", ErrMarketSort
	}
}

// MarketFilter is what the market shows: a gift and traits ("" for any)
// in an order.
type MarketFilter struct {
	Kind     GiftKind
	Model    string
	Backdrop string
	Symbol   string
	Sort     MarketSort
}

// MarketKey is where a page of the market goes on from: the value it is
// ordered by and the ID of the last gift.
type MarketKey struct {
	Value int64
	ID    uuid.UUID
}

// Of is the key of g in the order of f.
func (f MarketFilter) Of(g *Gift) MarketKey {
	if f.Sort == MarketByNumber {
		return MarketKey{Value: int64(g.number), ID: g.id}
	}

	return MarketKey{Value: g.salePrice, ID: g.id}
}
