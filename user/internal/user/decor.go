package user

import (
	"errors"
	"time"
)

// Decor is how the site is dressed for the New Year: falling snow, a
// garland under the header, a hat on the logo and a fir tree under the
// menu. The administrators turn it on, turn it off or leave it to the
// calendar.
type Decor string

const (
	// DecorAuto dresses the site for the season, from December 15 to
	// January 14 — the "old" New Year — and is the default.
	DecorAuto Decor = "auto"
	DecorOn   Decor = "on"
	DecorOff  Decor = "off"
)

var ErrDecorInvalid = errors.New("decor must be auto, on or off")

func NewDecor(s string) (Decor, error) {
	switch d := Decor(s); d {
	case DecorAuto, DecorOn, DecorOff:
		return d, nil
	}

	return "", ErrDecorInvalid
}

func (d Decor) String() string {
	return string(d)
}

// FestiveAt says whether the site wears the decoration at now, by the
// calendar of Moscow, where the site's people meet the New Year.
func (d Decor) FestiveAt(now time.Time) bool {
	switch d {
	case DecorOn:
		return true
	case DecorOff:
		return false
	}

	_, month, date := now.In(moscow).Date()

	return month == time.December && date >= 15 || month == time.January && date <= 14
}

// moscow is UTC+3 all year long; a fixed zone needs no tzdata in the image.
var moscow = time.FixedZone("MSK", 3*60*60)
