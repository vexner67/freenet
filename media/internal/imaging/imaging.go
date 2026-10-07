// Package imaging is what avatars and photos share: reading an upload
// safely and writing JPEG without metadata.
package imaging

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

	jpegQuality = 85
)

var (
	ErrTooLarge      = errors.New("image is larger than 8 MB")
	ErrNotImage      = errors.New("not a JPEG, PNG, GIF or WebP image")
	ErrTooManyPixels = errors.New("image dimensions are too large")
)

// Picture is a decoded upload as it is meant to be seen: its bounds and
// the rectangles given to Scale are of the upright picture.
type Picture struct {
	img         image.Image
	orientation orientation
}

// Decode reads an upload. It checks the header first: decoding a pixel
// bomb, a small file that claims a huge picture, would eat the memory.
func Decode(data []byte) (Picture, error) {
	if len(data) > MaxUploadBytes {
		return Picture{}, ErrTooLarge
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Picture{}, ErrNotImage
	}

	if cfg.Width*cfg.Height > maxPixels {
		return Picture{}, ErrTooManyPixels
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Picture{}, ErrNotImage
	}

	return Picture{img: img, orientation: orientationOf(data)}, nil
}

// Bounds are the upright picture's, from (0, 0).
func (p Picture) Bounds() image.Rectangle {
	w, h := p.img.Bounds().Dx(), p.img.Bounds().Dy()
	if p.orientation.transposed() {
		w, h = h, w
	}

	return image.Rect(0, 0, w, h)
}

// Scale draws the from part of the picture into an upright width×height
// JPEG. It scales the stored pixels and turns only the result, which is
// small. Encoding anew drops all metadata, EXIF with its location
// included.
func (p Picture) Scale(from image.Rectangle, width, height int) ([]byte, error) {
	if p.orientation.transposed() {
		width, height = height, width
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// JPEG has no transparency: show transparent parts as white.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), p.img, p.orientation.storedRect(from, p.img.Bounds()), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, p.orientation.turn(dst), &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
