package user

import (
	"errors"
	"slices"
	"time"
)

// PrimeStyle is how a member of Freenet Prime dresses their page, as the
// pages of the sites of the 2000s were: the colour of the name, the frame
// of the photo, the pattern behind the page and the mood by the status
// (the "настроение" of the blogs of the time). openvk has nothing like it.
// Others see it only while Prime lasts; the owner keeps it after.
type PrimeStyle struct {
	Name       PrimeName
	Frame      PrimeFrame
	Background PrimeBackground
	Mood       PrimeMood
}

// PrimeName is the colour of the name on the page; the site draws each.
type PrimeName string

// PrimeFrame is the frame of the photo on the page.
type PrimeFrame string

// PrimeBackground is the pattern behind the page.
type PrimeBackground string

// PrimeMood is the smiley by the status, "" for none.
type PrimeMood string

// The choices, the default first: the amethyst of Prime, no pattern, no
// mood. The site keeps their names and pictures (primeNames, primeFrames,
// primeBackgrounds, primeMoods in labels.js; a test compares them).
var (
	PrimeNames       = []PrimeName{"amethyst", "ruby", "emerald", "sapphire", "gold", "rainbow"}
	PrimeFrames      = []PrimeFrame{"amethyst", "gold", "silver", "glitter", "hearts", "stars"}
	PrimeBackgrounds = []PrimeBackground{"", "stars", "hearts", "checks", "clouds", "flowers", "sparkles"}
	PrimeMoods       = []PrimeMood{"", "happy", "love", "cool", "wink", "sad", "angry", "sleepy"}
)

var ErrPrimeStyleInvalid = errors.New("unknown style of the page")

// DefaultPrimeStyle is the page as Prime dresses it untouched: the
// amethyst name and frame.
func DefaultPrimeStyle() PrimeStyle {
	return PrimeStyle{Name: PrimeNames[0], Frame: PrimeFrames[0]}
}

// NewPrimeStyle checks each choice; "" for the name and the frame is the
// default. ErrPrimeStyleInvalid.
func NewPrimeStyle(name, frame, background, mood string) (PrimeStyle, error) {
	s := PrimeStyle{Name: PrimeName(name), Frame: PrimeFrame(frame), Background: PrimeBackground(background), Mood: PrimeMood(mood)}.filled()

	if !slices.Contains(PrimeNames, s.Name) || !slices.Contains(PrimeFrames, s.Frame) ||
		!slices.Contains(PrimeBackgrounds, s.Background) || !slices.Contains(PrimeMoods, s.Mood) {
		return PrimeStyle{}, ErrPrimeStyleInvalid
	}

	return s, nil
}

// IsDefault tells whether s changes nothing.
func (s PrimeStyle) IsDefault() bool {
	return s.filled() == DefaultPrimeStyle()
}

// filled is s with the default name and frame for unset ones, as of a
// user restored without a style.
func (s PrimeStyle) filled() PrimeStyle {
	if s.Name == "" {
		s.Name = PrimeNames[0]
	}
	if s.Frame == "" {
		s.Frame = PrimeFrames[0]
	}

	return s
}

// PrimeStyle is how u dressed their page; like wallet, storage alone
// changes it.
func (u *User) PrimeStyle() PrimeStyle {
	return u.primeStyle.filled()
}

// WithPrimeStyle is u with the style s, for storage that keeps users
// whole.
func (u *User) WithPrimeStyle(s PrimeStyle) *User {
	changed := *u
	changed.primeStyle = s

	return &changed
}

// shownPrimeStyle is the style others see at now: only while Prime lasts.
func (u *User) shownPrimeStyle(now time.Time) *PrimeStyle {
	if !u.IsPrimeAt(now) {
		return nil
	}

	s := u.PrimeStyle()

	return &s
}

// CheckPrimeStyle refuses a page without Prime at now any style but the
// default one, which is always allowed back.
func (u *User) CheckPrimeStyle(s PrimeStyle, now time.Time) error {
	if !s.IsDefault() && !u.IsPrimeAt(now) {
		return ErrPrimeOnly
	}

	return nil
}
