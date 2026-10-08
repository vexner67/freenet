package club

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

const maxAddressLen = 32

var (
	ErrAddressTooLong = errors.New("club address is too long")
	ErrAddressInvalid = errors.New("club address contains invalid characters")
	ErrAddressTaken   = errors.New("club address is already taken")

	// As a nickname: a letter first, so an address never looks like the
	// ID that a community without one goes by (/club/<id>).
	addressPattern = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9_]*[A-Za-z0-9])?$`)
)

// Address is the short name of a community's page, /club/<address>, unique
// whatever the case; "" means none, and the page goes by the ID.
type Address string

func NewAddress(s string) (Address, error) {
	if s == "" {
		return "", nil
	}

	if utf8.RuneCountInString(s) > maxAddressLen {
		return "", ErrAddressTooLong
	}

	if !addressPattern.MatchString(s) {
		return "", ErrAddressInvalid
	}

	return Address(s), nil
}

func (a Address) String() string {
	return string(a)
}
