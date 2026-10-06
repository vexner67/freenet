package avatar_test

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/media/internal/imaging"
)

// picture is a w×h image: red on the left half, blue on the right.
func picture(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func encode(t *testing.T, img image.Image, format string) []byte {
	t.Helper()

	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		t.Fatalf("stored image must be JPEG: %s, %v", format, err)
	}

	return img
}

func TestMake(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			images, err := avatar.Make(encode(t, picture(300, 200), format))
			if err != nil {
				t.Fatalf("Make: %v", err)
			}

			large, small := decode(t, images.Large), decode(t, images.Small)
			if large.Bounds().Dx() != 400 || large.Bounds().Dy() != 400 || small.Bounds().Dx() != 100 {
				t.Fatalf("sizes: %v, %v", large.Bounds(), small.Bounds())
			}

			// The middle square of 300×200 keeps both halves: red left, blue right.
			r, _, b, _ := large.At(20, 200).RGBA()
			if r < b {
				t.Fatalf("left must stay red: r=%d b=%d", r, b)
			}
			r, _, b, _ = large.At(380, 200).RGBA()
			if b < r {
				t.Fatalf("right must stay blue: r=%d b=%d", r, b)
			}
		})
	}
}

func TestMake_Transparency(t *testing.T) {
	t.Parallel()

	images, err := avatar.Make(encode(t, image.NewRGBA(image.Rect(0, 0, 100, 100)), "png"))
	if err != nil {
		t.Fatal(err)
	}

	if r, g, b, _ := decode(t, images.Small).At(50, 50).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Fatalf("transparent must become white, got %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestMake_Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"not an image", []byte("hello, i am a text file"), imaging.ErrNotImage},
		{"too small", encode(t, picture(40, 400), "png"), avatar.ErrTooSmall},
		{"too large file", make([]byte, imaging.MaxUploadBytes+1), imaging.ErrTooLarge},
		{"pixel bomb", pngHeader(20000, 20000), imaging.ErrTooManyPixels},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := avatar.Make(tt.data); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

// pngHeader is a tiny file that claims huge dimensions, like a pixel bomb.
func pngHeader(w, h int) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1)))
	data := buf.Bytes()

	// IHDR width and height are big-endian at bytes 16-23.
	put := func(at, v int) {
		data[at], data[at+1], data[at+2], data[at+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
	}
	put(16, w)
	put(20, h)
	// The IHDR checksum covers its type and data (bytes 12-28).
	put(29, int(crc32.ChecksumIEEE(data[12:29])))

	return data
}
