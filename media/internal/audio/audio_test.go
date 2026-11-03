package audio_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/vexner67/freenet/media/internal/audio"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{name: "tags first", data: []byte("ID3\x04\x00rest")},
		{name: "a frame first", data: []byte{0xFF, 0xFB, 0x90, 0x64}},
		{name: "empty", data: nil, want: audio.ErrEmpty},
		{name: "too big", data: append([]byte("ID3"), make([]byte, audio.MaxBytes)...), want: audio.ErrTooBig},
		{name: "a picture", data: []byte("\x89PNG\r\n\x1a\n"), want: audio.ErrNotMP3},
		{name: "a reserved layer", data: []byte{0xFF, 0xE0, 0x00}, want: audio.ErrNotMP3},
		{name: "text", data: bytes.Repeat([]byte("a"), 10), want: audio.ErrNotMP3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := audio.Check(tt.data); !errors.Is(err, tt.want) {
				t.Fatalf("Check = %v, want %v", err, tt.want)
			}
		})
	}
}
