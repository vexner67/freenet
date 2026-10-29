package user

import (
	"errors"
	"time"
)

// The rules of the treasury ("Казна"): every vote paid to a page comes out
// of it and every vote spent goes back into it. It has no bottom — the site
// pays whatever its rules say — and what it holds is only how much more it
// has taken than paid since it was made, below zero when it paid more.
const (
	// TreasuryStart is what the treasury held when it was made.
	TreasuryStart = 25_000
	// BonusAge is how old a page must be to take a bonus.
	BonusAge = 7 * 24 * time.Hour
	// MaxAmount is the most votes any one number the administrators set
	// may be — a price, a prize, a bet, a bonus. It is not a rule of the
	// economy, which has none (the treasury has no bottom), but a bound of
	// arithmetic: an amount times a multiplier of up to MaxMultiplier still
	// fits in int64.
	MaxAmount = 1_000_000_000_000
	// MaxMoved is the most votes one move of votes people make may carry —
	// a transfer, a deposit, a stake, a sale, a payment into an account —
	// as many as an administrator has (EndlessVotes). Wherever such an
	// amount is multiplied, it goes through MulDiv.
	MaxMoved = EndlessVotes
	// MaxPayout is the most one win or one product of votes pays (MulDiv),
	// so that a balance it is added to stays within int64.
	MaxPayout = 1_000_000_000_000_000_000
	// MaxMultiplier is the biggest multiplier the administrators set, in
	// hundredths: ×10000.
	MaxMultiplier = 1_000_000
)

// SettledBefore is when the pages registered before it count as old enough
// for the bonus and the mine whatever their age: the first pages of the
// site, a day short of a week when the mine opened, need not wait.
var SettledBefore = time.Date(2026, time.October, 21, 21, 0, 0, 0, time.UTC)

var (
	ErrBonusTaken  = errors.New("the bonus is taken today")
	ErrBonusNotYet = errors.New("the bonus is for pages a week old with a filled profile")
)

// Treasury is the votes of the site that are no one's: Votes — what it has
// taken more than paid, Outstanding — what the vouchers not used yet
// promise, and Held — the votes on the accounts of the pages, the rest of
// the economy.
type Treasury struct {
	Votes       int64
	Outstanding int64
	Held        int64
}

// BonusDay is the day of now a bonus is taken on, by the calendar of
// Moscow, at midnight UTC.
func BonusDay(now time.Time) time.Time {
	y, m, d := now.In(moscow).Date()

	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// CheckBonusAt is ErrBonusNotYet unless u may take a bonus at now: a page
// BonusAge old (or registered before SettledBefore) with a filled profile,
// neither deleted nor banned.
func (u *User) CheckBonusAt(now time.Time) error {
	registered := RegisteredAt(u.id)
	if !u.IsProfileCompleted() || u.IsDeactivated() || u.IsBannedAt(now) ||
		(now.Sub(registered) < BonusAge && !registered.Before(SettledBefore)) {
		return ErrBonusNotYet
	}

	return nil
}
