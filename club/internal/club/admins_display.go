package club

import "errors"

var ErrInvalidAdminsDisplay = errors.New("invalid admins display")

// AdminsDisplay is what the page of a community shows of those who run
// it, openvk's "Список админов": only the creator, all the admins or
// nothing.
type AdminsDisplay string

const (
	AdminsDisplayCreator AdminsDisplay = "creator"
	AdminsDisplayAll     AdminsDisplay = "all"
	AdminsDisplayNone    AdminsDisplay = "none"
)

func NewAdminsDisplay(s string) (AdminsDisplay, error) {
	switch d := AdminsDisplay(s); d {
	case AdminsDisplayCreator, AdminsDisplayAll, AdminsDisplayNone:
		return d, nil
	default:
		return "", ErrInvalidAdminsDisplay
	}
}

func (d AdminsDisplay) String() string {
	return string(d)
}
