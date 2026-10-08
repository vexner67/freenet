package club

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const maxDescriptionLen = 1000

var ErrDescriptionTooLong = errors.New("club description is too long")

// Description says what a community is about: plain text with "\n" line
// breaks, trimmed; empty is fine.
type Description string

func NewDescription(s string) (Description, error) {
	// Normalise line breaks so length and storage do not depend on the client.
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if utf8.RuneCountInString(s) > maxDescriptionLen {
		return "", ErrDescriptionTooLong
	}

	return Description(s), nil
}

func (d Description) String() string {
	return string(d)
}
