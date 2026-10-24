package user

import (
	"time"

	"github.com/google/uuid"
)

const (
	// OnlineFor is how long after their last visit a person counts as
	// online ("Онлайн"), as on openvk.
	OnlineFor = 5 * time.Minute

	// absenceShown is how long openvk tells anyone when a person was last
	// online; "Кто видит мою неактивность" hides only a longer absence.
	absenceShown = 29 * 24 * time.Hour
)

// LastSeen is when the person last visited the site ("был в сети"), the
// zero time if never.
func (u *User) LastSeen() time.Time {
	return u.lastSeen
}

// MarkSeen records a visit at at; an older one than known changes nothing,
// so that visits told out of order keep the last.
func (u *User) MarkSeen(at time.Time) {
	if at.After(u.lastSeen) {
		u.lastSeen = at
	}
}

// IsOnline tells whether the person visited within OnlineFor of now.
func (u *User) IsOnline(now time.Time) bool {
	return !u.lastSeen.IsZero() && now.Sub(u.lastSeen) < OnlineFor
}

// ListedProfile is the person in a list of people at now: what anyone
// may see (PublicProfile) and whether they are online ("Онлайн" by a
// name, as on openvk), with the Prime style that dresses their name and
// posts. A list does not know its viewer, so only a page open to everyone
// says it is online; when the person last visited, a list never tells.
func (u *User) ListedProfile(now time.Time) PublicProfile {
	if u.IsBannedAt(now) {
		return u.bannedProfile(now)
	}

	p := u.PublicProfile()
	p.Prime = u.IsPrimeAt(now)
	p.PrimeStyle = u.shownPrimeStyle(now)
	p.Online = !u.IsDeactivated() && u.privacy.PageRead == AudienceEveryone && !u.privacy.Closed && u.IsOnline(now) &&
		!u.invisibleAt(now)

	return p
}

// lastSeenFor is when viewerID (uuid.Nil: anonymous), tied to u by t,
// may see u last visited, the zero time if not: the owner and the
// audience of "Кто видит мою неактивность" always, the others while the
// absence is short, as on openvk. A deleted page tells nothing, nor an
// invisible one ("Невидимка" of Prime) to others.
func (u *User) lastSeenFor(viewerID uuid.UUID, t Ties, now time.Time) time.Time {
	if u.IsDeactivated() || viewerID != u.id && u.invisibleAt(now) {
		return time.Time{}
	}

	if viewerID == u.id || u.privacy.OnlineRead.admits(viewerID, t) || now.Sub(u.lastSeen) < absenceShown {
		return u.lastSeen
	}

	return time.Time{}
}
