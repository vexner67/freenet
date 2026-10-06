package photo_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/vexner67/freenet/media/internal/imaging"
	"github.com/vexner67/freenet/media/internal/photo"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{G: 200, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func size(t *testing.T, data []byte) (int, int) {
	t.Helper()

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		t.Fatalf("stored photo must be JPEG: %s, %v", format, err)
	}

	return cfg.Width, cfg.Height
}

func TestMake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		w, h           int
		largeW, largeH int
		smallW, smallH int
	}{
		{"landscape is scaled down whole", 2560, 1440, 1280, 720, 640, 360},
		{"portrait keeps its proportions", 1500, 3000, 640, 1280, 320, 640},
		{"small is not enlarged", 300, 200, 300, 200, 300, 200},
		{"between the sizes", 1000, 800, 1000, 800, 640, 512},
		{"a thin strip stays at least a pixel", 6400, 2, 1280, 1, 640, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			images, err := photo.Make(encodePNG(t, tt.w, tt.h))
			if err != nil {
				t.Fatal(err)
			}

			if w, h := size(t, images.Of(photo.Large)); w != tt.largeW || h != tt.largeH {
				t.Fatalf("large = %dx%d, want %dx%d", w, h, tt.largeW, tt.largeH)
			}
			if w, h := size(t, images.Of(photo.Small)); w != tt.smallW || h != tt.smallH {
				t.Fatalf("small = %dx%d, want %dx%d", w, h, tt.smallW, tt.smallH)
			}
		})
	}
}

func TestMake_Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		err  error
	}{
		{"not an image", []byte("hello, i am a text file"), imaging.ErrNotImage},
		{"too large file", make([]byte, imaging.MaxUploadBytes+1), imaging.ErrTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := photo.Make(tt.data); !errors.Is(err, tt.err) {
				t.Fatalf("Make = %v, want %v", err, tt.err)
			}
		})
	}
}
