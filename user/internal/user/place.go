package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const maxPlaceLen = 64

var ErrPlaceTooLong = errors.New("place is too long")

// Place names a town, as "Родной город" or "Город": one line, trimmed,
// possibly empty.
type Place string

func NewPlace(s string) (Place, error) {
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > maxPlaceLen {
		return "", ErrPlaceTooLong
	}

	return Place(s), nil
}

func (p Place) String() string {
	return string(p)
}
