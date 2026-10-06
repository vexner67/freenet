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
		{"empty is fine, photos may say it all", "", "", nil},
		{"only spaces and breaks", " \n\t\r\n ", "", nil},
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

	p, err := post.New(author, textOnly(t, "привет"), now)
	if err != nil {
		t.Fatal(err)
	}

	if p.CreatedAt().Location() != time.UTC || p.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", p.CreatedAt())
	}
	if !p.CanBeDeletedBy(author) || p.CanBeDeletedBy(uuid.New()) {
		t.Fatal("only the author may delete")
	}

	later, err := post.New(author, textOnly(t, "ещё"), now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= p.ID().String() {
		t.Fatal("IDs must grow, walls are ordered by them")
	}
}

// textOnly is the content of a post without photos.
func textOnly(t *testing.T, text string) post.Content {
	t.Helper()

	c, err := post.NewContent(post.Text(text), nil)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestNewPhotos(t *testing.T) {
	t.Parallel()

	a, b := uuid.New(), uuid.New()
	ten := make([]uuid.UUID, 10)
	for i := range ten {
		ten[i] = uuid.New()
	}

	tests := []struct {
		name string
		ids  []uuid.UUID
		err  error
	}{
		{"none", nil, nil},
		{"two in order", []uuid.UUID{b, a}, nil},
		{"ten", ten, nil},
		{"eleven", append(ten, uuid.New()), post.ErrTooManyPhotos},
		{"the same twice", []uuid.UUID{a, b, a}, post.ErrDuplicatePhoto},
		{"zero id", []uuid.UUID{uuid.Nil}, post.ErrInvalidPhoto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			photos, err := post.NewPhotos(tt.ids)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewPhotos = %v, want %v", err, tt.err)
			}
			if err == nil && len(photos) != len(tt.ids) {
				t.Fatalf("photos = %v", photos)
			}
		})
	}
}

func TestNewContent(t *testing.T) {
	t.Parallel()

	photos, err := post.NewPhotos([]uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		text   post.Text
		photos post.Photos
		err    error
	}{
		{"text", "привет", nil, nil},
		{"photos only", "", photos, nil},
		{"both", "смотрите", photos, nil},
		{"nothing", "", nil, post.ErrTextEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := post.NewContent(tt.text, tt.photos)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewContent = %v, want %v", err, tt.err)
			}
			if err == nil && (c.Text() != tt.text || len(c.Photos()) != len(tt.photos)) {
				t.Fatalf("content = %+v", c)
			}
		})
	}
}
