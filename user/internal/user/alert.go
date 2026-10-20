package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	// maxAlertLength bounds the label: a longer one would push the page down.
	maxAlertLength = 1000
	// maxAlertLines bounds its lines: a list of achievements, not a wall.
	maxAlertLines = 20
)

var (
	ErrAlertTooLong = errors.New("the label is too long")
	ErrAlertLines   = errors.New("the label has too many lines")
)

// Alert is the orange label of openvk ("user-alert") the administrators of
// the site put above the name of a page, with words of their own: the
// marking of a foreign agent or anything else. The empty Alert is no
// label.
type Alert string

// NewAlert is the label of s, which may be a list of lines (achievements,
// one under another); "" takes the label away. Spaces at the ends of the
// lines and empty lines go: they would only make the plaque taller.
func NewAlert(s string) (Alert, error) {
	s, err := visibleText(strings.ReplaceAll(s, "\r\n", "\n"))
	if err != nil {
		return "", err
	}

	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) > maxAlertLines {
		return "", ErrAlertLines
	}

	s = strings.Join(lines, "\n")

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
