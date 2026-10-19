package user

import (
	"errors"
	"unicode/utf8"
)

const maxSurnameLen = 100

var (
	ErrSurnameEmpty   = errors.New("surname is empty")
	ErrSurnameTooLong = errors.New("surname is too long")
)

type Surname string

func NewSurname(s string) (Surname, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if utf8.RuneCountInString(s) > maxSurnameLen {
		return "", ErrSurnameTooLong
	}

	return Surname(s), nil
}

func (n Surname) String() string {
	return string(n)
}
