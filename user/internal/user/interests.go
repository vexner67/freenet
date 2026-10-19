package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// maxAboutLen is openvk's for each of "Личная информация".
const maxAboutLen = 1000

var ErrAboutTooLong = errors.New("text is too long")

// Interests are openvk's "Личная информация": everything optional.
type Interests struct {
	Interests About
	Music     About
	Films     About
	Shows     About
	Books     About
	Quotes    About
	Games     About
	About     About
}

// About is a free text of the page: trimmed, with "\n" line breaks.
type About string

func NewAbout(s string) (About, error) {
	s, err := visibleText(strings.ReplaceAll(s, "\r\n", "\n"))
	if err != nil {
		return "", err
	}

	if utf8.RuneCountInString(s) > maxAboutLen {
		return "", ErrAboutTooLong
	}

	return About(s), nil
}

func (a About) String() string {
	return string(a)
}
