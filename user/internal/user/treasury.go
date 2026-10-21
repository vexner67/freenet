package user

import (
	"errors"
	"time"
)

// The rules of the treasury ("Казна"): every vote paid to a page comes out
// of it and every vote spent goes back into it, so votes appear only as the
// code says.
const (
	// TreasuryStart is what the treasury holds when it is made.
	TreasuryStart = 25_000
	// TreasuryReserve is what the treasury keeps for the wins and the
	// bonuses: vouchers take only what is above it, and the bonus shrinks
	// below it.
	TreasuryReserve = 20_000
	// MemberMint is what a new member brings into the treasury, once: a
	// page older than BonusAge with a filled profile.
	MemberMint = 1_000
	// MaxBonus is the daily bonus while the treasury holds the reserve.
	MaxBonus = 10
	// BonusAge is how old a page must be to take a bonus.
	BonusAge = 7 * 24 * time.Hour
	// MaxWin is the most one round of the casino wins.
	MaxWin = 1_000
	// WinShare is the part of the treasury one round may win at most: 1/20.
	WinShare = 20
	// MaxCasinoReturn is the most the casino gives back on average, in
	// percent.
	MaxCasinoReturn = 85
)

var (
	ErrTreasuryEmpty = errors.New("the treasury is empty")
	ErrTreasuryLow   = errors.New("the treasury cannot pay such a win now")
	ErrVoucherCover  = errors.New("the treasury cannot cover this voucher")
	ErrBonusTaken    = errors.New("the bonus is taken today")
	ErrBonusNotYet   = errors.New("the bonus is for pages a week old with a filled profile")
)

// Treasury is the votes of the site that are no one's: Votes held, Minted
// by new members, Outstanding — promised by the vouchers not used yet.
type Treasury struct {
	Votes       int64
	Minted      int64
	Outstanding int64
}

// Free is what vouchers may still promise: the votes above the reserve and
// what the vouchers promise already.
func (t Treasury) Free() int64 {
	return max(0, t.Votes-TreasuryReserve-t.Outstanding)
}

// Bonus is the daily bonus now: MaxBonus while the treasury holds the
// reserve, less as much as it is below, but never below 1.
func (t Treasury) Bonus() int64 {
	if t.Votes >= TreasuryReserve {
		return MaxBonus
	}

	return max(1, MaxBonus*max(0, t.Votes)/TreasuryReserve)
}

// MaxWin is the most one round may win now: MaxWin, but not above 1/20 of
// the treasury.
func (t Treasury) MaxWin() int64 {
	return min(MaxWin, max(0, t.Votes)/WinShare)
}

// CheckWin is ErrTreasuryLow for a win above MaxWin.
func (t Treasury) CheckWin(votes int64) error {
	if votes > t.MaxWin() {
		return ErrTreasuryLow
	}

	return nil
}

// CheckVoucher is ErrVoucherCover when the vouchers would promise more than
// is free.
func (t Treasury) CheckVoucher(v *Voucher) error {
	if v.Votes()*int64(v.UsagesLeft()) > t.Free() {
		return ErrVoucherCover
	}

	return nil
}

// BonusDay is the day of now a bonus is taken on, by the calendar of
// Moscow, at midnight UTC.
func BonusDay(now time.Time) time.Time {
	y, m, d := now.In(moscow).Date()

	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// CheckBonusAt is ErrBonusNotYet unless u may take a bonus at now: a page
// BonusAge old with a filled profile, neither deleted nor banned. Such a
// page is a member, who brings MemberMint into the treasury.
func (u *User) CheckBonusAt(now time.Time) error {
	if !u.IsProfileCompleted() || u.IsDeactivated() || u.IsBannedAt(now) ||
		now.Sub(RegisteredAt(u.id)) < BonusAge {
		return ErrBonusNotYet
	}

	return nil
}
