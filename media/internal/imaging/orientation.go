package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
)

// orientation is the EXIF Orientation of a picture: how the stored pixels
// turn upright. Phones store a photo as the sensor saw it and only mark it
// "turn me", so ignoring the mark lays such a photo on its side.
type orientation uint16

const (
	upright     orientation = 1
	maxRotation orientation = 8

	// From orientation 5 on, the stored picture is turned by a quarter:
	// its width is the height of the upright one.
	firstTransposed orientation = 5

	jpegSOI     = 0xD8
	jpegEOI     = 0xD9
	jpegSOS     = 0xDA
	jpegAPP1    = 0xE1
	tagOrient   = 0x0112
	typeShort   = 3
	ifdEntryLen = 12
)

func (o orientation) transposed() bool {
	return o >= firstTransposed
}

// toStored maps a point of the upright picture to the stored one, which is
// w×h. Points are corners between pixels: for the pixel at (x, y) pass
// w-1 and h-1 instead.
func (o orientation) toStored(x, y, w, h int) (int, int) {
	switch o {
	case 2: // mirrored
		return w - x, y
	case 3: // upside down
		return w - x, h - y
	case 4: // mirrored upside down
		return x, h - y
	case 5: // mirrored along the main diagonal
		return y, x
	case 6: // turned left: turn right to see
		return y, h - x
	case 7: // mirrored along the other diagonal
		return w - y, h - x
	case 8: // turned right: turn left to see
		return w - y, x
	default:
		return x, y
	}
}

// storedRect is r of the upright picture in the stored one, whose bounds
// are b.
func (o orientation) storedRect(r, b image.Rectangle) image.Rectangle {
	x0, y0 := o.toStored(r.Min.X, r.Min.Y, b.Dx(), b.Dy())
	x1, y1 := o.toStored(r.Max.X, r.Max.Y, b.Dx(), b.Dy())

	return image.Rect(x0, y0, x1, y1).Add(b.Min)
}

// turn returns src upright. It runs on the scaled picture, so it copies
// thousands of pixels, not the millions of the upload.
func (o orientation) turn(src *image.RGBA) *image.RGBA {
	if o == upright {
		return src
	}

	w, h := src.Rect.Dx(), src.Rect.Dy()
	if o.transposed() {
		w, h = h, w
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			sx, sy := o.toStored(x, y, src.Rect.Dx()-1, src.Rect.Dy()-1)
			copy(dst.Pix[dst.PixOffset(x, y):][:4], src.Pix[src.PixOffset(sx, sy):][:4])
		}
	}

	return dst
}

// orientationOf reads the orientation from the EXIF of a JPEG. Other
// formats, a JPEG without EXIF and a broken EXIF are taken as upright.
func orientationOf(data []byte) orientation {
	if len(data) < 2 || data[0] != 0xFF || data[1] != jpegSOI {
		return upright
	}

	// Segments: 0xFF, a marker, a big-endian length that counts itself,
	// then the content. EXIF is the APP1 segment, before the image data.
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return upright
		}

		marker := data[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == jpegSOS || marker == jpegEOI {
			return upright
		}

		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return upright
		}

		if marker == jpegAPP1 {
			if o, ok := exifOrientation(data[i+4 : i+2+size]); ok {
				return o
			}
		}

		i += 2 + size
	}

	return upright
}

// exifOrientation finds the Orientation tag in the first IFD of an APP1
// segment. Every offset is checked: the bytes come from the uploader.
func exifOrientation(segment []byte) (orientation, bool) {
	tiff, ok := bytes.CutPrefix(segment, []byte("Exif\x00\x00"))
	if !ok || len(tiff) < 8 {
		return 0, false
	}

	var order binary.ByteOrder
	switch string(tiff[:4]) {
	case "II*\x00":
		order = binary.LittleEndian
	case "MM\x00*":
		order = binary.BigEndian
	default:
		return 0, false
	}

	ifd := uint64(order.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > uint64(len(tiff)) {
		return 0, false
	}

	entries := int(order.Uint16(tiff[ifd:]))
	for e := int(ifd) + 2; entries > 0 && e+ifdEntryLen <= len(tiff); e, entries = e+ifdEntryLen, entries-1 {
		if order.Uint16(tiff[e:]) != tagOrient {
			continue
		}

		// A SHORT whose value sits in the first two bytes of the field.
		if order.Uint16(tiff[e+2:]) != typeShort {
			return 0, false
		}

		o := orientation(order.Uint16(tiff[e+8:]))
		if o < upright || o > maxRotation {
			return 0, false
		}

		return o, true
	}

	return 0, false
}
