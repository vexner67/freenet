package user

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const maxDeactivationReasonLength = 1000

var (
	// ErrDeactivated: the page is deleted, so nobody may follow it, write
	// on its wall or to its owner.
	ErrDeactivated = errors.New("this page is deleted")
	// ErrNotDeactivated: only a deleted page can be restored.
	ErrNotDeactivated = errors.New("this page is not deleted")
	// ErrRestoreExpired: the page is due to be erased and stays deleted.
	ErrRestoreExpired            = errors.New("this page can no longer be restored")
	ErrDeactivationReasonTooLong = errors.New("reason is too long")
)

// DeactivationReason is why the owner deleted the page, as openvk asks:
// one of its reasons or the owner's own words; it may be empty.
type DeactivationReason string

func NewDeactivationReason(s string) (DeactivationReason, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if utf8.RuneCountInString(s) > maxDeactivationReasonLength {
		return "", ErrDeactivationReasonTooLong
	}

	return DeactivationReason(s), nil
}

func (r DeactivationReason) String() string {
	return string(r)
}

// Deactivation is the deletion of a page by its owner ("Удаление
// страницы" on openvk): the page is hidden and its owner may restore it
// until PurgeAt, when its data is erased for good. The zero value is a
// page that is not deleted.
type Deactivation struct {
	At      time.Time
	PurgeAt time.Time
	Reason  DeactivationReason
}
