// Package account holds what auth knows about the person signing in.
package account

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const maxEmailLen = 254

var ErrInvalidEmail = errors.New("invalid email")

// Email is a sign-in address in canonical form: trimmed and lower-cased,
// so "Ann@Example.com" and "ann@example.com" share codes, limits and account.
type Email string

func NewEmail(s string) (Email, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if s == "" || utf8.RuneCountInString(s) > maxEmailLen {
		return "", ErrInvalidEmail
	}

	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", ErrInvalidEmail
	}

	return Email(s), nil
}

func (e Email) String() string {
	return string(e)
}
