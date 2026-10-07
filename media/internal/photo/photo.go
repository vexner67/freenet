// Package photo turns an uploaded picture into the photos of a post.
package photo

import (
	"errors"

	"github.com/vexner67/freenet/media/internal/imaging"
)

const (
	// Longest sides of the stored photos: the large one for viewing on its
	// own, the small one twice as big as the tiles of a post, for
	// high-density screens.
	largeSide = 1280
	smallSide = 640
)

// Size is which of the stored photos to read.
type Size string

const (
	Large Size = "large" // opened on its own
	Small Size = "small" // in the post
)

var ErrNotFound = errors.New("photo not found")

// Images are the stored photos, JPEG-encoded.
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

// Make scales an upload to both sizes, keeping its proportions: unlike an
// avatar, a photo is shown whole.
func Make(data []byte) (Images, error) {
	img, err := imaging.Decode(data)
	if err != nil {
		return Images{}, err
	}

	large, err := fit(img, largeSide)
	if err != nil {
		return Images{}, err
	}

	small, err := fit(img, smallSide)
	if err != nil {
		return Images{}, err
	}

	return Images{Large: large, Small: small}, nil
}

// fit scales img down so that its longer side is at most side; a smaller
// picture keeps its size, enlarging would only blur it.
func fit(img imaging.Picture, side int) ([]byte, error) {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()

	if longest := max(width, height); longest > side {
		width = max(1, width*side/longest)
		height = max(1, height*side/longest)
	}

	return img.Scale(b, width, height)
}
