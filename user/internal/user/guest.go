package user

import (
	"time"

	"github.com/google/uuid"
)

// GuestsFor is how long a visit stays among the guests of a page.
const GuestsFor = 30 * 24 * time.Hour

// MaxGuests is the most guests a page of them holds.
const MaxGuests = 50

// Guest is who visited a page and when they did last.
type Guest struct {
	GuestID   uuid.UUID
	VisitedAt time.Time
}

// LeavesTraceOn tells whether a visit of u to the page of owner at now is
// kept among its guests: not one's own page, and not by a page without a
// filled profile (it has no name to show), deleted, banned or invisible
// with Prime.
func (u *User) LeavesTraceOn(owner *User, now time.Time) bool {
	return u.id != owner.id && u.profile.Nickname != "" && !owner.IsDeactivated() &&
		!u.IsDeactivated() && !u.IsBannedAt(now) && !u.invisibleAt(now)
}
