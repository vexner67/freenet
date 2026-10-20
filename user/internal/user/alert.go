package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// maxAlertLength bounds the label: a longer one would push the page down.
const maxAlertLength = 1000

var (
	ErrAlertTooLong = errors.New("the label is too long")
	// ErrAlertLines: the label is one line, as openvk shows it.
	ErrAlertLines = errors.New("the label must be one line")
)

// Alert is the orange label of openvk ("user-alert") the administrators of
// the site put above the name of a page, with words of their own: the
// marking of a foreign agent or anything else. The empty Alert is no
// label.
type Alert string

// NewAlert is the label of s, one line; "" takes the label away.
func NewAlert(s string) (Alert, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if strings.ContainsAny(s, "\r\n") {
		return "", ErrAlertLines
	}

	if utf8.RuneCountInString(s) > maxAlertLength {
		return "", ErrAlertTooLong
	}

	return Alert(s), nil
}

func (a Alert) String() string {
	return string(a)
}

// Alert is the label the administrators of the site put on the page, ""
// for none.
func (u *User) Alert() Alert {
	return u.alert
}

// WithAlert is u with the label put or, with "", taken, for storage that
// keeps users whole.
func (u *User) WithAlert(alert Alert) *User {
	changed := *u
	changed.alert = alert

	return &changed
}
