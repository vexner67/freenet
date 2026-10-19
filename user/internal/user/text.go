package user

import (
	"errors"
	"strings"
	"unicode"
)

// ErrInvisibleText is a text of the page with characters that show
// nothing: the fillers people take for a blank name or status.
var ErrInvisibleText = errors.New("text contains invisible characters")

// invisible are characters that show nothing and say nothing in a text of
// the page: format and control characters, zero-width spaces and the
// fillers of the Hangul and Braille scripts. Joiners and direction marks
// are not among them — emoji and right-to-left scripts need them.
func invisible(r rune) bool {
	switch r {
	case '\n', '\r', '\t',
		'\u061c',           // Arabic letter mark
		'\u200c', '\u200d', // zero-width non-joiner and joiner
		'\u200e', '\u200f': // left-to-right and right-to-left marks
		return false
	case '\u034f', // combining grapheme joiner
		'\u115f', '\u1160', '\u3164', '\uffa0', // Hangul fillers
		'\u17b4', '\u17b5', // Khmer inherent vowels
		'\u2800': // Braille blank
		return true
	}

	// Tags spell the flags of England, Scotland and Wales.
	if r >= '\U000e0020' && r <= '\U000e007f' {
		return false
	}

	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cc, r)
}

// blank are characters that show nothing at the ends of a text.
func blank(r rune) bool {
	return unicode.IsSpace(r) || invisible(r) || r == '\u200c' || r == '\u200d' || r == '\u200e' || r == '\u200f'
}

// visibleText refuses a text with invisible characters and trims what
// shows nothing at its ends, so that a text of only such characters is
// empty.
func visibleText(s string) (string, error) {
	if strings.IndexFunc(s, invisible) >= 0 {
		return "", ErrInvisibleText
	}

	s = strings.TrimFunc(s, blank)
	// Variation selectors left alone show nothing either.
	if strings.IndexFunc(s, func(r rune) bool { return !unicode.Is(unicode.Variation_Selector, r) }) < 0 {
		return "", nil
	}

	return s, nil
}
