package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	// MaxExtraFields is openvk's "users.max-fields".
	MaxExtraFields       = 7
	maxExtraFieldNameLen = 50
)

var (
	ErrTooManyExtraFields   = errors.New("at most 7 additional fields")
	ErrExtraFieldNameEmpty  = errors.New("field name is empty")
	ErrExtraFieldNameLong   = errors.New("field name is too long")
	ErrExtraFieldTextEmpty  = errors.New("field text is empty")
	ErrInvalidExtraFieldPos = errors.New("invalid field place")
)

// FieldPlace is where an additional field shows: with the contacts or with
// the interests.
type FieldPlace string

const (
	FieldPlaceContacts  FieldPlace = "contacts"
	FieldPlaceInterests FieldPlace = "interests"
)

func NewFieldPlace(s string) (FieldPlace, error) {
	switch FieldPlace(s) {
	case FieldPlaceContacts, FieldPlaceInterests:
		return FieldPlace(s), nil
	default:
		return "", ErrInvalidExtraFieldPos
	}
}

func (p FieldPlace) String() string {
	return string(p)
}

// ExtraField is openvk's "Дополнительно": a field of the page its owner
// names, shown with the contacts or the interests.
type ExtraField struct {
	Name  string
	Text  About
	Place FieldPlace
}

func NewExtraField(name, text string, place FieldPlace) (ExtraField, error) {
	name, err := visibleText(name)
	if err != nil {
		return ExtraField{}, err
	}

	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ExtraField{}, ErrExtraFieldNameEmpty
	}

	if utf8.RuneCountInString(name) > maxExtraFieldNameLen {
		return ExtraField{}, ErrExtraFieldNameLong
	}

	about, err := NewAbout(text)
	if err != nil {
		return ExtraField{}, err
	}

	if about == "" {
		return ExtraField{}, ErrExtraFieldTextEmpty
	}

	return ExtraField{Name: name, Text: about, Place: place}, nil
}
