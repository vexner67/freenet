package club

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const maxNameLen = 64

var (
	ErrNameEmpty   = errors.New("club name is empty")
	ErrNameTooLong = errors.New("club name is too long")
)

// Name is what a community is called: one line, trimmed.
type Name string

func NewName(s string) (Name, error) {
	s = strings.Join(strings.Fields(s), " ")

	if s == "" {
		return "", ErrNameEmpty
	}

	if utf8.RuneCountInString(s) > maxNameLen {
		return "", ErrNameTooLong
	}

	return Name(s), nil
}

func (n Name) String() string {
	return string(n)
}
