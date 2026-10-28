package world

import "errors"

var ErrFlag = errors.New("invalid flag")

// Colors is how many colors the palettes of flags and of states have; a
// color is its number, and the site keeps what it looks like.
const Colors = 24

// Pattern is how a flag is divided.
type Pattern string

const (
	PatternPlain     Pattern = "plain"
	PatternBicolorH  Pattern = "bicolor_h"
	PatternBicolorV  Pattern = "bicolor_v"
	PatternTricolorH Pattern = "tricolor_h"
	PatternTricolorV Pattern = "tricolor_v"
	PatternCross     Pattern = "cross"
	PatternNordic    Pattern = "nordic"
	PatternDiagonal  Pattern = "diagonal"
	PatternBorder    Pattern = "border"
	PatternTriangle  Pattern = "triangle"
	PatternCircle    Pattern = "circle"
)

// patternColors is how many colors each pattern has.
var patternColors = map[Pattern]int{
	PatternPlain: 1, PatternBicolorH: 2, PatternBicolorV: 2, PatternTricolorH: 3, PatternTricolorV: 3,
	PatternCross: 2, PatternNordic: 2, PatternDiagonal: 2, PatternBorder: 2, PatternTriangle: 2, PatternCircle: 2,
}

// Emblem is a sign on a flag.
type Emblem string

const EmblemNone Emblem = ""

var emblems = map[Emblem]bool{
	EmblemNone: true, "star": true, "sun": true, "moon": true, "crown": true, "anchor": true, "tower": true,
	"sword": true, "tree": true, "wave": true, "wheat": true, "shield": true, "hammer": true,
}

// EmblemPlace is where the emblem stands.
type EmblemPlace string

const (
	EmblemCenter EmblemPlace = "center"
	EmblemCanton EmblemPlace = "canton"
)

// Flag is a flag made of a pattern, its colors and an emblem: no pictures
// to upload, nothing to look over.
type Flag struct {
	pattern     Pattern
	colors      []int
	emblem      Emblem
	emblemColor int
	emblemAt    EmblemPlace
}

// FlagParts are the parts of a flag.
type FlagParts struct {
	Pattern     Pattern
	Colors      []int
	Emblem      Emblem
	EmblemColor int
	EmblemAt    EmblemPlace
}

// NewFlag checks the parts: as many colors as the pattern has, every one
// of the palette.
func NewFlag(p FlagParts) (Flag, error) {
	n, ok := patternColors[p.Pattern]
	if !ok || len(p.Colors) != n || !emblems[p.Emblem] || !inPalette(p.EmblemColor) {
		return Flag{}, ErrFlag
	}

	for _, c := range p.Colors {
		if !inPalette(c) {
			return Flag{}, ErrFlag
		}
	}

	at := p.EmblemAt
	switch {
	case p.Emblem == EmblemNone:
		at = EmblemCenter
	case at != EmblemCenter && at != EmblemCanton:
		return Flag{}, ErrFlag
	}

	return Flag{
		pattern: p.Pattern, colors: append([]int(nil), p.Colors...), emblem: p.Emblem,
		emblemColor: p.EmblemColor, emblemAt: at,
	}, nil
}

func inPalette(c int) bool {
	return c >= 0 && c < Colors
}

// Parts are what the flag is made of.
func (f Flag) Parts() FlagParts {
	return FlagParts{
		Pattern: f.pattern, Colors: append([]int(nil), f.colors...), Emblem: f.emblem,
		EmblemColor: f.emblemColor, EmblemAt: f.emblemAt,
	}
}

// RestoreFlag rebuilds a flag from storage; parts that are no longer
// known give a plain flag.
func RestoreFlag(p FlagParts) Flag {
	f, err := NewFlag(p)
	if err != nil {
		return Flag{pattern: PatternPlain, colors: []int{0}, emblemAt: EmblemCenter}
	}

	return f
}
