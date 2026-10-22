package user

import (
	"errors"
	"time"
)

// Freenet Prime is the subscription of the site (ours; openvk has none):
// bought for votes, which go to the treasury, by the month, three months
// or the year, for oneself or as a present. It gives a mark by the name, a
// golden frame of the photo, a theme and gifts of its own, takes the ads
// off the feed, hides when its owner is online if they wish, lets them
// watch stories unseen and makes transfers and sales of stickers cheaper.

const (
	// MaxPrimePrice is the dearest a period of Prime may be, in votes.
	MaxPrimePrice = MaxAmount
)

// DefaultPrimePrices are what Prime costs until the administrators say
// otherwise: half the bonuses of a month a month, cheaper by the quarter
// and the year.
var DefaultPrimePrices = PrimePrices{Month: 150, Quarter: 400, Year: 1500}

var (
	ErrPrimePeriodInvalid = errors.New("unknown period of prime")
	ErrPrimePriceInvalid  = errors.New("the price of prime must be at least 1 vote")
	// ErrPrimeOnly: what only those with Prime may have — its gifts, its
	// theme, being invisible.
	ErrPrimeOnly = errors.New("this is for freenet prime only")
)

// PrimePeriod is how long a purchase of Prime lasts.
type PrimePeriod string

const (
	PrimeMonth   PrimePeriod = "month"
	PrimeQuarter PrimePeriod = "quarter"
	PrimeYear    PrimePeriod = "year"
)

func NewPrimePeriod(s string) (PrimePeriod, error) {
	switch p := PrimePeriod(s); p {
	case PrimeMonth, PrimeQuarter, PrimeYear:
		return p, nil
	default:
		return "", ErrPrimePeriodInvalid
	}
}

func (p PrimePeriod) String() string {
	return string(p)
}

// Days is how many days the period adds.
func (p PrimePeriod) Days() int {
	switch p {
	case PrimeQuarter:
		return 90
	case PrimeYear:
		return 365
	default:
		return 30
	}
}

// PrimePrices are what each period of Prime costs, in votes.
type PrimePrices struct {
	Month   int64
	Quarter int64
	Year    int64
}

func NewPrimePrices(month, quarter, year int64) (PrimePrices, error) {
	for _, price := range []int64{month, quarter, year} {
		if price < 1 || price > MaxPrimePrice {
			return PrimePrices{}, ErrPrimePriceInvalid
		}
	}

	return PrimePrices{Month: month, Quarter: quarter, Year: year}, nil
}

// Of is what period costs.
func (p PrimePrices) Of(period PrimePeriod) int64 {
	switch period {
	case PrimeQuarter:
		return p.Quarter
	case PrimeYear:
		return p.Year
	default:
		return p.Month
	}
}

// Prime is the subscription of a page: until when it lasts (the zero
// time: never bought) and whether its owner is invisible while it does.
type Prime struct {
	Until     time.Time
	Invisible bool
}

// ActiveAt tells whether the subscription lasts at now.
func (p Prime) ActiveAt(now time.Time) bool {
	return now.Before(p.Until)
}

// Extended is until when the subscription lasts after period is added at
// now: to its end, or to now when it has ended.
func (p Prime) Extended(period PrimePeriod, now time.Time) time.Time {
	from := p.Until
	if from.Before(now) {
		from = now
	}

	return from.AddDate(0, 0, period.Days())
}

// Prime is the subscription of u; like wallet, storage alone changes it.
func (u *User) Prime() Prime {
	return u.prime
}

// IsPrimeAt tells whether u has Prime at now; a deleted page has none.
func (u *User) IsPrimeAt(now time.Time) bool {
	return !u.IsDeactivated() && u.prime.ActiveAt(now)
}

// WithPrime is u with the subscription p, for storage that keeps users
// whole.
func (u *User) WithPrime(p Prime) *User {
	changed := *u
	changed.prime = p

	return &changed
}

// invisibleAt tells whether u hides when they are online at now.
func (u *User) invisibleAt(now time.Time) bool {
	return u.prime.Invisible && u.IsPrimeAt(now)
}

// CheckInvisible refuses invisibility to a page without Prime at now;
// becoming visible is always allowed.
func (u *User) CheckInvisible(invisible bool, now time.Time) error {
	if invisible && !u.IsPrimeAt(now) {
		return ErrPrimeOnly
	}

	return nil
}

// TransferFeeAt is what a transfer of votes costs u on top at now in e:
// nothing with Prime.
func (u *User) TransferFeeAt(e Economy, votes int64, now time.Time) int64 {
	if u.IsPrimeAt(now) {
		return 0
	}

	return e.TransferFeeOf(votes)
}

// StickerTaxAt is what the treasury takes of a sale of a pack of u, its
// author, for price at now in e: the tax of Prime with it.
func (u *User) StickerTaxAt(e Economy, price int64, now time.Time) int64 {
	return e.StickerTaxOf(price, u.IsPrimeAt(now))
}
