package user

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxGiftCaptionLength is how long the words with a gift may be.
const MaxGiftCaptionLength = 250

// MaxGiftPrice is the dearest an administrator may make a gift, in votes.
const MaxGiftPrice = 1_000_000

var (
	ErrUnknownGift        = errors.New("unknown gift")
	ErrGiftCaptionTooLong = errors.New("gift caption is too long")
	ErrGiftPriceInvalid   = errors.New("the price of a gift must be between 0 and 1000000")
	ErrGiftNotFound       = errors.New("gift not found")
	// ErrGiftRefused: the page is not shown to the sender or either of the
	// two has blacklisted the other.
	ErrGiftRefused = errors.New("you cannot send a gift to this user")
	// ErrGiftsHidden: the page or its gifts are not shown to the viewer
	// ("Кому видны мои подарки").
	ErrGiftsHidden = errors.New("this user's gifts are hidden")
	// ErrCannotDeleteGift: a gift is deleted by the one it was sent to.
	ErrCannotDeleteGift = errors.New("only the owner of a gift can delete it")
)

// GiftKind names a gift of the catalogue: what is sent.
type GiftKind string

// GiftCollection is a shelf of the catalogue, openvk's "Коллекции".
type GiftCollection struct {
	ID string
	// Price is what each of its gifts costs the sender, in votes, unless
	// the administrators priced it otherwise (GiftPrices); 0 for a free
	// one ("Бесплатно").
	Price GiftPrice
	Gifts []GiftKind
}

// giftCatalog is all there is to send. openvk's gifts are drawn and priced
// by its administrators; here the catalogue is fixed, and so are the
// prices the gifts have until the administrators change them: the flowers
// are free, so that anyone can send a gift, and the rest costs votes,
// which the daily bonus and vouchers give — a gift of a collection is a
// day or a week of bonuses. "freenet" are the site's own gifts, the
// dearest, last. The pictures are the site's: it knows a gift by this name.
var giftCatalog = []GiftCollection{
	{ID: "flowers", Gifts: []GiftKind{"rose", "tulip", "bouquet", "sunflower", "cactus"}},
	{ID: "sweets", Price: 10, Gifts: []GiftKind{"cake", "candy", "chocolate", "icecream", "cupcake"}},
	{ID: "holidays", Price: 30, Gifts: []GiftKind{"present", "balloons", "heart", "bear", "star"}},
	{ID: "freenet", Price: 70, Gifts: []GiftKind{"freenet_crystal", "freenet_coin", "freenet_ring", "freenet_crown", "freenet_heart", "freenet_cup"}},
}

// dearGifts cost more than their collection: the crown and the cup are a
// month of bonuses.
var dearGifts = map[GiftKind]GiftPrice{
	"freenet_crown": 300,
	"freenet_cup":   300,
}

// GiftCatalog are the collections of gifts, in the order they are shown.
func GiftCatalog() []GiftCollection {
	catalog := make([]GiftCollection, len(giftCatalog))
	for i, c := range giftCatalog {
		catalog[i] = GiftCollection{ID: c.ID, Price: c.Price, Gifts: slices.Clone(c.Gifts)}
	}

	return catalog
}

// NewGiftKind takes only a gift of the catalogue.
func NewGiftKind(s string) (GiftKind, error) {
	for _, c := range giftCatalog {
		if slices.Contains(c.Gifts, GiftKind(s)) {
			return GiftKind(s), nil
		}
	}

	return "", ErrUnknownGift
}

func (k GiftKind) String() string {
	return string(k)
}

// DefaultPrice is what sending the gift costs, in votes, until the
// administrators price it otherwise: its own price or that of its
// collection.
func (k GiftKind) DefaultPrice() GiftPrice {
	if price, ok := dearGifts[k]; ok {
		return price
	}

	for _, c := range giftCatalog {
		if slices.Contains(c.Gifts, k) {
			return c.Price
		}
	}

	return 0
}

// GiftPrice is what a gift costs its sender, in votes: 0 (free) to
// MaxGiftPrice.
type GiftPrice int64

func NewGiftPrice(n int64) (GiftPrice, error) {
	if n < 0 || n > MaxGiftPrice {
		return 0, ErrGiftPriceInvalid
	}

	return GiftPrice(n), nil
}

func (p GiftPrice) Int64() int64 {
	return int64(p)
}

// GiftPrices are the prices the administrators of the site set, by gift;
// the rest cost what their collections do.
type GiftPrices map[GiftKind]GiftPrice

// Of is what kind costs.
func (p GiftPrices) Of(kind GiftKind) GiftPrice {
	if price, ok := p[kind]; ok {
		return price
	}

	return kind.DefaultPrice()
}

// GiftCaption is what the sender says with a gift ("Ваше сообщение"); it
// may be empty.
type GiftCaption string

func NewGiftCaption(s string) (GiftCaption, error) {
	s = strings.TrimSpace(s)

	if utf8.RuneCountInString(s) > MaxGiftCaptionLength {
		return "", ErrGiftCaptionTooLong
	}

	return GiftCaption(s), nil
}

func (c GiftCaption) String() string {
	return string(c)
}

// Gift is a gift someone sent to a page, as openvk's "Подарки". The
// sender is uuid.Nil once their page is erased: the gift stays.
type Gift struct {
	id          uuid.UUID
	recipientID uuid.UUID
	senderID    uuid.UUID
	kind        GiftKind
	caption     GiftCaption
	anonymous   bool
}

// GiftFrom is the gift senderID, tied to u by t, sends to u. A deleted
// page takes no gifts, a block shuts them both ways, and a page that does
// not show itself to the sender (hidden or closed) takes none from them,
// as on openvk. One may send a gift to oneself, as there.
func (u *User) GiftFrom(senderID uuid.UUID, t Ties, kind GiftKind, caption GiftCaption, anonymous bool) (*Gift, error) {
	if u.IsDeactivated() {
		return nil, ErrDeactivated
	}

	if senderID != u.id && (t.Blocked || !u.access(senderID, t, false).Page) {
		return nil, ErrGiftRefused
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Gift{
		id:          id,
		recipientID: u.id,
		senderID:    senderID,
		kind:        kind,
		caption:     caption,
		anonymous:   anonymous,
	}, nil
}

// RestoreGift rebuilds a gift from storage.
func RestoreGift(id, recipientID, senderID uuid.UUID, kind GiftKind, caption GiftCaption, anonymous bool) *Gift {
	return &Gift{
		id:          id,
		recipientID: recipientID,
		senderID:    senderID,
		kind:        kind,
		caption:     caption,
		anonymous:   anonymous,
	}
}

func (g *Gift) ID() uuid.UUID          { return g.id }
func (g *Gift) RecipientID() uuid.UUID { return g.recipientID }
func (g *Gift) SenderID() uuid.UUID    { return g.senderID }
func (g *Gift) Kind() GiftKind         { return g.kind }
func (g *Gift) Caption() GiftCaption   { return g.caption }
func (g *Gift) Anonymous() bool        { return g.anonymous }

// SentAt is the time of the gift's UUIDv7.
func (g *Gift) SentAt() time.Time {
	sec, nsec := g.id.Time().UnixTime()

	return time.Unix(sec, nsec).UTC()
}

// SignedFor tells whether viewerID (uuid.Nil: anonymous) is told who sent
// the gift and what they said: anyone, unless it was sent anonymously
// ("Анонимно") — then only the one it was sent to and the sender.
func (g *Gift) SignedFor(viewerID uuid.UUID) bool {
	if !g.anonymous {
		return true
	}

	return viewerID != uuid.Nil && (viewerID == g.recipientID || viewerID == g.senderID)
}

// CanBeDeletedBy: a gift is its recipient's to delete.
func (g *Gift) CanBeDeletedBy(userID uuid.UUID) bool {
	return userID == g.recipientID
}

// Notifies tells whether the recipient hears of the gift: not of one's
// own to oneself.
func (g *Gift) Notifies() bool {
	return g.senderID != uuid.Nil && g.senderID != g.recipientID
}
