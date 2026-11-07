package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// BackdropSide is a side of the page a backdrop picture is on.
type BackdropSide string

const (
	BackdropLeft  BackdropSide = "left"
	BackdropRight BackdropSide = "right"
	// BackdropCover is a photo of one's own behind the whole page, a
	// thing of Freenet Prime.
	BackdropCover BackdropSide = "cover"
)

var ErrInvalidBackdropSide = errors.New("invalid backdrop side")

func NewBackdropSide(s string) (BackdropSide, error) {
	switch BackdropSide(s) {
	case BackdropLeft, BackdropRight, BackdropCover:
		return BackdropSide(s), nil
	default:
		return "", ErrInvalidBackdropSide
	}
}

func (s BackdropSide) String() string {
	return string(s)
}

// Backdrop is openvk's "Фон страницы": two photos the media service
// keeps under the user, shown on the left and on the right of the page,
// and, with Prime, one behind the whole page (Cover); uuid.Nil for a side
// without one. Photos are public, as the avatar.
type Backdrop struct {
	Left  uuid.UUID
	Right uuid.UUID
	Cover uuid.UUID
}

func (u *User) Backdrop() Backdrop {
	return u.backdrop
}

// SetBackdrop shows photoID (uuid.Nil: none) on side, returning what the
// side showed before.
func (u *User) SetBackdrop(side BackdropSide, photoID uuid.UUID) (previous uuid.UUID) {
	switch side {
	case BackdropLeft:
		previous, u.backdrop.Left = u.backdrop.Left, photoID
	case BackdropRight:
		previous, u.backdrop.Right = u.backdrop.Right, photoID
	case BackdropCover:
		previous, u.backdrop.Cover = u.backdrop.Cover, photoID
	}

	return previous
}

// CheckBackdrop refuses a page without Prime at now a photo behind the
// whole page; taking one away is always allowed.
func (u *User) CheckBackdrop(side BackdropSide, photoID uuid.UUID, now time.Time) error {
	if side == BackdropCover && photoID != uuid.Nil && !u.IsPrimeAt(now) {
		return ErrPrimeOnly
	}

	return nil
}

// shownBackdrop is the backdrop others see at now: the photo behind the
// whole page only while Prime lasts.
func (u *User) shownBackdrop(now time.Time) Backdrop {
	b := u.backdrop
	if !u.IsPrimeAt(now) {
		b.Cover = uuid.Nil
	}

	return b
}
