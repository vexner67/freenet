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

// Decode reads an upload. It checks the header first: decoding a pixel
// bomb, a small file that claims a huge picture, would eat the memory.
func Decode(data []byte) (image.Image, error) {
	if len(data) > MaxUploadBytes {
		return nil, ErrTooLarge
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}

	if cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooManyPixels
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}

	return img, nil
}

// Scale draws the from part of src into a width×height JPEG. Encoding
// anew drops all metadata, EXIF with its location included.
func Scale(src image.Image, from image.Rectangle, width, height int) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// JPEG has no transparency: show transparent parts as white.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, from, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
