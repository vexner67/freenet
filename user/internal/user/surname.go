package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const maxSurnameLen = 100

var (
	ErrSurnameEmpty   = errors.New("surname is empty")
	ErrSurnameTooLong = errors.New("surname is too long")
)

type Surname string

func NewSurname(s string) (Surname, error) {
	s = strings.TrimSpace(s)

	if s == "" {
		return "", ErrSurnameTooLong
	}

	if utf8.RuneCountInString(s) > maxSurnameLen {
		return "", ErrSurnameEmpty
	}

	return Surname(s), nil
}

func (n Surname) String() string {
	return string(n)
}
