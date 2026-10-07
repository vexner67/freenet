package imaging_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/vexner67/freenet/media/internal/imaging"
)

// storedJPEG is a 40×20 JPEG, white with its top-left quarter red, marked
// with the EXIF orientation o (none when 0).
func storedJPEG(t testing.TB, o uint16) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			c := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if x < 20 && y < 10 {
				c = color.RGBA{R: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}

	if o == 0 {
		return buf.Bytes()
	}

	return withEXIF(buf.Bytes(), exifSegment(binary.LittleEndian, o))
}

// exifSegment is an APP1 segment whose first IFD holds only Orientation.
func exifSegment(order binary.AppendByteOrder, o uint16) []byte {
	tiff := []byte("II*\x00")
	if order == binary.BigEndian {
		tiff = []byte("MM\x00*")
	}
	tiff = order.AppendUint32(tiff, 8) // the IFD right after the header
	tiff = order.AppendUint16(tiff, 1) // one entry
	tiff = order.AppendUint16(tiff, 0x0112)
	tiff = order.AppendUint16(tiff, 3) // SHORT
	tiff = order.AppendUint32(tiff, 1) // one value
	tiff = order.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0)       // the rest of the value field
	tiff = append(tiff, 0, 0, 0, 0) // no next IFD

	content := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xFF, 0xE1}
	segment = binary.BigEndian.AppendUint16(segment, uint16(len(content)+2))

	return append(segment, content...)
}

// withEXIF puts the segment right after the start of the JPEG, where
// cameras put it.
func withEXIF(data, segment []byte) []byte {
	out := append([]byte{}, data[:2]...)
	out = append(out, segment...)

	return append(out, data[2:]...)
}

// redCorner tells which quarter of a picture is red.
func redCorner(t *testing.T, data []byte) (image.Point, string) {
	t.Helper()

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	b := img.Bounds()
	corners := map[string]image.Point{
		"top-left":     {b.Dx() / 4, b.Dy() / 4},
		"top-right":    {b.Dx() * 3 / 4, b.Dy() / 4},
		"bottom-left":  {b.Dx() / 4, b.Dy() * 3 / 4},
		"bottom-right": {b.Dx() * 3 / 4, b.Dy() * 3 / 4},
	}

	red := ""
	for name, p := range corners {
		r, g, _, _ := img.At(p.X, p.Y).RGBA()
		if r > 0xC000 && g < 0x4000 {
			if red != "" {
				t.Fatalf("both %s and %s are red", red, name)
			}
			red = name
		}
	}

	return image.Pt(b.Dx(), b.Dy()), red
}

func TestOrientation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		orientation uint16
		size        image.Point
		red         string
	}{
		{0, image.Pt(40, 20), "top-left"},
		{1, image.Pt(40, 20), "top-left"},
		{2, image.Pt(40, 20), "top-right"},
		{3, image.Pt(40, 20), "bottom-right"},
		{4, image.Pt(40, 20), "bottom-left"},
		{5, image.Pt(20, 40), "top-left"},
		{6, image.Pt(20, 40), "top-right"},
		{7, image.Pt(20, 40), "bottom-right"},
		{8, image.Pt(20, 40), "bottom-left"},
		{9, image.Pt(40, 20), "top-left"}, // not an orientation: ignored
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.orientation)), func(t *testing.T) {
			t.Parallel()

			p, err := imaging.Decode(storedJPEG(t, tt.orientation))
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Bounds().Size(); got != tt.size {
				t.Fatalf("Bounds = %v, want %v", got, tt.size)
			}

			out, err := p.Scale(p.Bounds(), tt.size.X, tt.size.Y)
			if err != nil {
				t.Fatal(err)
			}
			if size, red := redCorner(t, out); size != tt.size || red != tt.red {
				t.Fatalf("got %v with %q red, want %v with %q red", size, red, tt.size, tt.red)
			}
		})
	}
}

// TestOrientationCrop crops a part of the upright picture: the part must be
// found in the stored pixels, which lie otherwise.
func TestOrientationCrop(t *testing.T) {
	t.Parallel()

	// Turned right to see: the upright picture is 20×40 with the red
	// quarter at the top right, x 10..20, y 0..20.
	p, err := imaging.Decode(storedJPEG(t, 6))
	if err != nil {
		t.Fatal(err)
	}

	isRed := func(rect image.Rectangle, at image.Point) bool {
		t.Helper()

		data, err := p.Scale(rect, 20, 20)
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		r, g, _, _ := img.At(at.X, at.Y).RGBA()
		return r > 0xC000 && g < 0x4000
	}

	top, bottom := image.Rect(0, 0, 20, 20), image.Rect(0, 20, 20, 40)
	if !isRed(top, image.Pt(15, 10)) || isRed(top, image.Pt(5, 10)) {
		t.Fatal("the top square must be red on the right and white on the left")
	}
	if isRed(bottom, image.Pt(15, 10)) || isRed(bottom, image.Pt(5, 10)) {
		t.Fatal("the bottom square must be white")
	}
}

func TestOrientationBigEndian(t *testing.T) {
	t.Parallel()

	data := withEXIF(storedJPEG(t, 0), exifSegment(binary.BigEndian, 8))

	p, err := imaging.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Bounds().Size(); got != image.Pt(20, 40) {
		t.Fatalf("Bounds = %v, want turned", got)
	}
}

// FuzzDecode feeds broken EXIF: offsets come from the uploader, so no
// value may panic or read outside the segment.
func FuzzDecode(f *testing.F) {
	f.Add(storedJPEG(f, 6))
	f.Add(withEXIF(storedJPEG(f, 0), []byte{0xFF, 0xE1, 0x00, 0x10, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', '*', 0, 0xFF, 0xFF, 0xFF, 0x7F}))
	f.Add(withEXIF(storedJPEG(f, 0), []byte{0xFF, 0xE1, 0xFF, 0xFF}))
	f.Add([]byte{0xFF, 0xD8, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := imaging.Decode(data)
		if err != nil {
			return
		}

		b := p.Bounds()
		if b.Dx() > 0 && b.Dy() > 0 {
			if _, err = p.Scale(b, 4, 4); err != nil {
				t.Fatal(err)
			}
		}
	})
}
