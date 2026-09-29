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

	if utf8.RuneCountInString(s) > maxSurnameLen {
		return "", ErrSurnameTooLong
	}

	return Surname(s), nil
}

func (n Surname) String() string {
	return string(n)
}
