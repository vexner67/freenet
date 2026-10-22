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
	// MaxBonus is the daily bonus.
	MaxBonus = 10
	// BonusAge is how old a page must be to take a bonus.
	BonusAge = 7 * 24 * time.Hour
	// MaxWin is the most one round of the casino wins.
	MaxWin = 1_000
	// MaxCasinoReturn is the most the casino gives back on average, in
	// percent.
	MaxCasinoReturn = 85
)

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
// BonusAge old with a filled profile, neither deleted nor banned.
func (u *User) CheckBonusAt(now time.Time) error {
	if !u.IsProfileCompleted() || u.IsDeactivated() || u.IsBannedAt(now) ||
		now.Sub(RegisteredAt(u.id)) < BonusAge {
		return ErrBonusNotYet
	}

	return nil
}
