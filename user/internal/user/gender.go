package user

import "errors"

type Pronouns string

const (
	PronounsHe  Pronouns = "he"
	PronounsShe Pronouns = "she"
)

var ErrInvalidPronouns = errors.New("invalid pronouns")

func NewPronouns(s string) (Pronouns, error) {
	switch Pronouns(s) {
	case PronounsHe, PronounsShe:
		return Pronouns(s), nil
	default:
		return "", ErrInvalidPronouns
	}
}

func (p Pronouns) String() string {
	return string(p)
}
