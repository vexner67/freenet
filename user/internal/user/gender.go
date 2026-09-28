package user

import "errors"

type Gender string

const (
	GenderNone   Gender = ""
	GenderMale   Gender = "male"
	GenderFemale Gender = "female"
)

var ErrInvalidGender = errors.New("invalid gender")

func NewGender(s string) (Gender, error) {
	switch Gender(s) {
	case GenderNone, GenderMale, GenderFemale:
		return Gender(s), nil
	default:
		return "", ErrInvalidGender
	}
}

func (g Gender) String() string {
	return string(g)
}
