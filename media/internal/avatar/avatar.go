// Package avatar turns an uploaded picture into profile photos.
package avatar

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"

	// Decoders for the accepted formats.
	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"
)

const (
	MaxUploadBytes = 8 << 20

	// maxPixels rejects images that are small as files but huge decoded.
	maxPixels = 50_000_000
	minSide   = 50

	// Sides of the stored squares: twice the size they are shown at, so
	// they stay sharp on high-density screens.
	largeSide = 400
	smallSide = 100

	jpegQuality = 85
)

// Size is which of the stored squares to read.
type Size string

const (
	Large Size = "large" // the profile photo
	Small Size = "small" // next to posts and in lists
)

var (
	ErrTooLarge      = errors.New("image is larger than 8 MB")
	ErrNotImage      = errors.New("not a JPEG, PNG, GIF or WebP image")
	ErrTooManyPixels = errors.New("image dimensions are too large")
	ErrTooSmall      = errors.New("image is smaller than 50x50")
	ErrNotFound      = errors.New("avatar not found")
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
// Re-encoding drops all metadata, EXIF with its location included.
func Make(data []byte) (Images, error) {
	if len(data) > MaxUploadBytes {
		return Images{}, ErrTooLarge
	}

	// Check the header first: decoding a pixel bomb would eat the memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Images{}, ErrNotImage
	}

	if cfg.Width*cfg.Height > maxPixels {
		return Images{}, ErrTooManyPixels
	}

	if min(cfg.Width, cfg.Height) < minSide {
		return Images{}, ErrTooSmall
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Images{}, ErrNotImage
	}

	square := centerSquare(img.Bounds())

	large, err := scale(img, square, largeSide)
	if err != nil {
		return Images{}, err
	}

	small, err := scale(img, square, smallSide)
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

func scale(src image.Image, from image.Rectangle, side int) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, side, side))

	// JPEG has no transparency: show transparent parts as white.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, from, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
