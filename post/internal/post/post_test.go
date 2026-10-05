package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  post.Text
		err   error
	}{
		{"plain", "привет", "привет", nil},
		{"trimmed, windows line breaks", "  раз\r\nдва \n", "раз\nдва", nil},
		{"emoji count as one character", strings.Repeat("👋", 1000), post.Text(strings.Repeat("👋", 1000)), nil},
		{"empty", "", "", post.ErrTextEmpty},
		{"only spaces and breaks", " \n\t\r\n ", "", post.ErrTextEmpty},
		{"too long", strings.Repeat("я", 1001), "", post.ErrTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewText(tt.input)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewText = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestPost(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	now := time.Date(2026, 10, 4, 18, 30, 0, 123456789, time.FixedZone("MSK", 3*3600))

	p, err := post.New(author, "привет", now)
	if err != nil {
		t.Fatal(err)
	}

	if p.CreatedAt().Location() != time.UTC || p.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", p.CreatedAt())
	}
	if !p.CanBeDeletedBy(author) || p.CanBeDeletedBy(uuid.New()) {
		t.Fatal("only the author may delete")
	}

	later, err := post.New(author, "ещё", now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= p.ID().String() {
		t.Fatal("IDs must grow, walls are ordered by them")
	}
}
