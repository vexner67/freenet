// Package captcha draws the digits a person has to read before a code is
// mailed. They are lines with no text in the picture, each bent and turned
// on its own under lines of noise, so reading them takes more than parsing
// the SVG.
package captcha

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/vexner67/freenet/pkg/errs"
)

const (
	// Length is how many digits a picture shows.
	Length = 5

	width      = 170
	height     = 54
	cellWidth  = 30
	noiseLines = 7
	// jitter bends a digit, in the units of glyphs, tenths of a cell unit.
	jitter = 12
	// tilt is how far a digit turns either way, in degrees.
	tilt = 22
)

// glyphs are the digits as strokes in a box of 10 by 14.
var glyphs = [10][][2]float64{
	{{5, 1}, {1, 4}, {1, 10}, {5, 13}, {9, 10}, {9, 4}, {5, 1}},
	{{3, 3}, {6, 1}, {6, 13}},
	{{1, 3}, {5, 1}, {9, 4}, {1, 13}, {9, 13}},
	{{1, 1}, {9, 1}, {5, 6}, {9, 9}, {6, 13}, {1, 12}},
	{{7, 13}, {7, 1}, {1, 9}, {9, 9}},
	{{9, 1}, {2, 1}, {1, 6}, {7, 6}, {9, 10}, {6, 13}, {1, 12}},
	{{8, 1}, {3, 4}, {1, 10}, {4, 13}, {8, 11}, {7, 7}, {2, 8}},
	{{1, 1}, {9, 1}, {4, 13}},
	{{5, 7}, {1, 4}, {5, 1}, {9, 4}, {1, 10}, {5, 13}, {9, 10}, {5, 7}},
	{{8, 6}, {3, 7}, {1, 3}, {5, 1}, {9, 4}, {7, 10}, {2, 13}},
}

// New is the digits of a new picture and the picture itself, an SVG.
func New() (answer, image string, err error) {
	digits, err := number(Length)
	if err != nil {
		return "", "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#eef1f5"/>`, width, height)

	for range noiseLines {
		if err = noise(&b); err != nil {
			return "", "", err
		}
	}

	for i, digit := range digits {
		if err = glyph(&b, byte(digit), float64(10+i*cellWidth)); err != nil {
			return "", "", err
		}
	}
	b.WriteString(`</svg>`)

	return digits, b.String(), nil
}

// number is n random decimal digits.
func number(n int) (string, error) {
	var b strings.Builder

	for range n {
		d, err := between(0, 9)
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + d))
	}

	return b.String(), nil
}

func between(low, high int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(high-low+1))
	if err != nil {
		return 0, errs.Wrap("rand.Int", err)
	}

	return low + n.Int64(), nil
}

func noise(b *strings.Builder) error {
	var at [6]int64

	for i, limit := range []int64{width, height, width, height, 150, 200} {
		n, err := between(0, limit)
		if err != nil {
			return err
		}
		at[i] = n
	}

	fmt.Fprintf(b, `<path d="M%d %dL%d %d" stroke="rgb(%d,%d,220)" fill="none"/>`, at[0], at[1], at[2], at[3], at[4], at[5])

	return nil
}

// glyph draws digit from x, turned about its middle, each point bent.
func glyph(b *strings.Builder, digit byte, x float64) error {
	degrees, err := between(-tilt, tilt)
	if err != nil {
		return err
	}
	lift, err := between(3, 10)
	if err != nil {
		return err
	}

	sin, cos := math.Sincos(float64(degrees) * math.Pi / 180)
	path := make([]string, 0, 12)

	for i, p := range glyphs[digit-'0'] {
		dx, err := between(-jitter, jitter)
		if err != nil {
			return err
		}
		dy, err := between(-jitter, jitter)
		if err != nil {
			return err
		}

		// Scaled to the height of the picture, about the middle of the box.
		px, py := (p[0]-5+float64(dx)/10)*2.2, (p[1]-7+float64(dy)/10)*2.4
		command := "L"
		if i == 0 {
			command = "M"
		}
		path = append(path, fmt.Sprintf("%s%.1f %.1f", command, x+12+px*cos-py*sin, float64(lift)+17+px*sin+py*cos))
	}

	fmt.Fprintf(b, `<path d="%s" stroke="#2f4f75" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" fill="none"/>`, strings.Join(path, ""))

	return nil
}
