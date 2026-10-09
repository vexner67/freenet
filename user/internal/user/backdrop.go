package user

import (
	"errors"

	"github.com/google/uuid"
)

// BackdropSide is a side of the page a backdrop picture is on.
type BackdropSide string

const (
	BackdropLeft  BackdropSide = "left"
	BackdropRight BackdropSide = "right"
)

var ErrInvalidBackdropSide = errors.New("invalid backdrop side")

func NewBackdropSide(s string) (BackdropSide, error) {
	switch BackdropSide(s) {
	case BackdropLeft, BackdropRight:
		return BackdropSide(s), nil
	default:
		return "", ErrInvalidBackdropSide
	}
}

func (s BackdropSide) String() string {
	return string(s)
}

// Backdrop is openvk's "Фон страницы": two photos the media service
// keeps under the user, shown on the left and on the right of the page;
// uuid.Nil for a side without one. Photos are public, as the avatar.
type Backdrop struct {
	Left  uuid.UUID
	Right uuid.UUID
}

func (u *User) Backdrop() Backdrop {
	return u.backdrop
}

// SetBackdrop shows photoID (uuid.Nil: none) on side, returning what the
// side showed before.
func (u *User) SetBackdrop(side BackdropSide, photoID uuid.UUID) (previous uuid.UUID) {
	if side == BackdropLeft {
		previous, u.backdrop.Left = u.backdrop.Left, photoID
	} else {
		previous, u.backdrop.Right = u.backdrop.Right, photoID
	}

	return previous
}
