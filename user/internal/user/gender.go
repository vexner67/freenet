package user

import "errors"

type Gender string

const (
	GenderNone   Gender = ""
	GenderMale   Gender = "male"
	GenderFemale Gender = "female"
	// GenderNeutral is openvk's third "Местоимения", "они/их".
	GenderNeutral Gender = "neutral"
)

var ErrInvalidGender = errors.New("invalid gender")

func NewGender(s string) (Gender, error) {
	switch Gender(s) {
	case GenderNone, GenderMale, GenderFemale, GenderNeutral:
		return Gender(s), nil
	default:
		return "", ErrInvalidGender
	}
}

func (g Gender) String() string {
	return string(g)
}
