package room

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTitleLen = 64

type Title string

func NewTitle(s string) (Title, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || s == "" || utf8.RuneCountInString(s) > maxTitleLen || strings.ContainsFunc(s, unicode.IsControl) || strings.ContainsAny(s, "\u2028\u2029") {
		return "", ErrInvalidTitle
	}
	return Title(s), nil
}

func (t Title) String() string { return string(t) }
