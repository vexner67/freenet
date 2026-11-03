// Package audio is a song of "Аудиозаписи" (openvk's audio), an MP3 file
// kept as uploaded and played from the site. Its artist, title and length
// the post service keeps; here it is bytes.
package audio

import (
	"bytes"
	"errors"
)

// MaxBytes is the largest song: a long MP3 of high quality fits.
const MaxBytes = 20 << 20

var (
	ErrNotFound = errors.New("audio not found")
	ErrEmpty    = errors.New("audio is empty")
	ErrTooBig   = errors.New("audio is larger than 20 MB")
	ErrNotMP3   = errors.New("audio is not an MP3 file")
)

// Check tells whether data may be kept as a song: an MP3 file, which
// starts with its ID3 tags or with the sync bits of an MPEG audio frame.
func Check(data []byte) error {
	switch {
	case len(data) == 0:
		return ErrEmpty
	case len(data) > MaxBytes:
		return ErrTooBig
	case !isMP3(data):
		return ErrNotMP3
	default:
		return nil
	}
}

func isMP3(data []byte) bool {
	if bytes.HasPrefix(data, []byte("ID3")) {
		return true
	}

	// Eleven set bits, then a layer that is not the reserved 00.
	return len(data) >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0 && data[1]&0x06 != 0
}
