package user

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const maxEmailLen = 254

var (
	ErrEmailEmpty   = errors.New("email is empty")
	ErrEmailTooLong = errors.New("email is too long")
	ErrEmailInvalid = errors.New("invalid email")
)

type Email string

func NewEmail(s string) (Email, error) {
	s = strings.TrimSpace(s)

	if s == "" {
		return "", ErrEmailEmpty
	}

	if utf8.RuneCountInString(s) > maxEmailLen {
		return "", ErrEmailTooLong
	}

	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", ErrEmailInvalid
	}

	return Email(s), nil
}

func (e Email) String() string {
	return string(e)
}
