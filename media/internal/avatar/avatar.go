// Package avatar turns an uploaded picture into profile photos.
package avatar

import (
	"errors"
	"image"

	"github.com/vexner67/freenet/media/internal/imaging"
)

const (
	minSide = 50

	// Sides of the stored squares: twice the size they are shown at, so
	// they stay sharp on high-density screens.
	largeSide = 400
	smallSide = 100
)

// Size is which of the stored squares to read.
type Size string

const (
	Large Size = "large" // the profile photo
	Small Size = "small" // next to posts and in lists
)

var (
	ErrTooSmall = errors.New("image is smaller than 50x50")
	ErrNotFound = errors.New("avatar not found")
)

// Images are the stored squares, JPEG-encoded.
type Images struct {
	Large []byte
	Small []byte
}

func (i Images) Of(size Size) []byte {
	if size == Small {
		return i.Small
	}

	return i.Large
}

// Make crops the middle square of an upload and scales it to both sizes.
func Make(data []byte) (Images, error) {
	img, err := imaging.Decode(data)
	if err != nil {
		return Images{}, err
	}

	if min(img.Bounds().Dx(), img.Bounds().Dy()) < minSide {
		return Images{}, ErrTooSmall
	}

	square := centerSquare(img.Bounds())

	large, err := imaging.Scale(img, square, largeSide, largeSide)
	if err != nil {
		return Images{}, err
	}

	small, err := imaging.Scale(img, square, smallSide, smallSide)
	if err != nil {
		return Images{}, err
	}

	return Images{Large: large, Small: small}, nil
}

func centerSquare(b image.Rectangle) image.Rectangle {
	side := min(b.Dx(), b.Dy())
	x := b.Min.X + (b.Dx()-side)/2
	y := b.Min.Y + (b.Dy()-side)/2

	return image.Rect(x, y, x+side, y+side)
}
