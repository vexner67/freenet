package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const maxNameLen = 100

var (
	ErrNameEmpty   = errors.New("name is empty")
	ErrNameTooLong = errors.New("name is too long")
)

type Name string

func NewName(s string) (Name, error) {
	s = strings.TrimSpace(s)

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
