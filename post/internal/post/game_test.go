package post_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewGameData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		err  error
	}{
		{"empty", "", nil},
		{"json", `{"level":3,"имя":"Аня"}`, nil},
		{"64 KB", strings.Repeat("x", post.MaxGameDataBytes), nil},
		{"over 64 KB", strings.Repeat("x", post.MaxGameDataBytes+1), post.ErrGameDataTooLarge},
		// The limit is in bytes: Cyrillic takes two each.
		{"over 64 KB of Cyrillic", strings.Repeat("я", post.MaxGameDataBytes/2+1), post.ErrGameDataTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewGameData(tt.in)
			if !errors.Is(err, tt.err) || (err == nil && got.String() != tt.in) {
				t.Fatalf("NewGameData = %d bytes, %v; want %v", len(got), err, tt.err)
			}
		})
	}
}

func TestNewGameScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int64
		err  error
	}{
		{"zero", 0, nil},
		{"score", 1500, nil},
		{"largest exact", 1<<53 - 1, nil},
		{"negative", -1, post.ErrInvalidGameScore},
		{"too large", 1 << 53, post.ErrInvalidGameScore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewGameScore(tt.in)
			if !errors.Is(err, tt.err) || (err == nil && got.Int64() != tt.in) {
				t.Fatalf("NewGameScore(%d) = %d, %v; want %v", tt.in, got, err, tt.err)
			}
		})
	}

	if !post.GameScore(10).Better(5) || post.GameScore(5).Better(5) {
		t.Fatal("Better")
	}
}
