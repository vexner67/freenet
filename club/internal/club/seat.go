package club

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// maxTitleLen is what openvk allows a comment on an admin.
const maxTitleLen = 36

var (
	ErrTitleTooLong = errors.New("title is too long")
	ErrNotManager   = errors.New("the user does not run the club")
	ErrTitleChanged = errors.New("the title was changed")
)

// Title is what an admin does in the club, in a few words shown under
// their name: openvk's "комментарий" to an admin. Empty for none.
type Title string

// NewTitle reads a title of one line: line breaks become spaces.
func NewTitle(s string) (Title, error) {
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > maxTitleLen {
		return "", ErrTitleTooLong
	}

	return Title(s), nil
}

func (t Title) String() string {
	return string(t)
}

// Seat is what a user is in a club and how the club shows it: the title
// of those who run it, and whether they are hidden ("Скрыт: Да" of
// openvk) and pass for a plain member to everyone but each other.
type Seat struct {
	Role   Role
	Title  Title
	Hidden bool
}

// Shown is the seat as someone who is viewer in the club sees it: a
// hidden admin is a member to those who do not run the club.
func (s Seat) Shown(viewer Role) Seat {
	if s.Hidden && !viewer.Manages() {
		return Seat{Role: RoleMember}
	}

	return s
}

// Seen tells whether someone who is viewer in the club sees that the seat
// runs it.
func (s Seat) Seen(viewer Role) bool {
	return s.Shown(viewer).Role.Manages()
}

// SeatChange is what a change of a seat names: its title, whether it
// is hidden, or both. What is nil stays as it is, so two people changing
// different things of one seat undo nothing of each other's.
//
// A title is one value, so two people writing it at once would undo each
// other's words: a change that names the title it started from (Was) is
// refused with ErrTitleChanged when the seat says another by now.
type SeatChange struct {
	Title  *Title
	Was    *Title
	Hidden *bool
}

// Outdated tells whether the change started from a title the seat, whose
// title is now, no longer has.
func (c SeatChange) Outdated(now Title) bool {
	return c.Title != nil && c.Was != nil && *c.Was != now
}

// CheckDescribe tells whether requester may give a title to someone who
// is target in the club, or hide them: those who run the club do it for
// each other, as in openvk.
func CheckDescribe(requester, target Role) error {
	if !requester.Manages() {
		return ErrCannotEdit
	}

	if !target.Manages() {
		return ErrNotManager
	}

	return nil
}
