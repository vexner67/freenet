package user

import (
	"errors"
	"unicode/utf8"
)

const maxStatusLength = 200

var (
	ErrStatusTooLong = errors.New("status is too long")
)

type Status string

func NewStatus(s string) (Status, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if utf8.RuneCountInString(s) > maxStatusLength {
		return "", ErrStatusTooLong
	}

	return Status(s), nil
}

func (s Status) String() string {
	return string(s)
}
