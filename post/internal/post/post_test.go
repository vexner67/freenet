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

	p, err := post.New(author, author, textOnly(t, "привет"), now)
	if err != nil {
		t.Fatal(err)
	}

	if p.CreatedAt().Location() != time.UTC || p.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", p.CreatedAt())
	}
	if !p.OnOwnWall() || p.WallID() != author {
		t.Fatalf("wall = %v, want the author's own", p.WallID())
	}
	if !p.CanBeDeletedBy(author) || p.CanBeDeletedBy(uuid.New()) {
		t.Fatal("only the author may delete a post on their own wall")
	}

	later, err := post.New(author, author, textOnly(t, "ещё"), now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= p.ID().String() {
		t.Fatal("IDs must grow, walls are ordered by them")
	}
}

func TestPost_OnSomeoneElsesWall(t *testing.T) {
	t.Parallel()

	owner, author, stranger := uuid.New(), uuid.New(), uuid.New()

	p, err := post.New(owner, author, textOnly(t, "привет"), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if p.OnOwnWall() || p.WallID() != owner || p.AuthorID() != author {
		t.Fatalf("wall = %v, author = %v", p.WallID(), p.AuthorID())
	}
	if !p.CanBeDeletedBy(author) || !p.CanBeDeletedBy(owner) {
		t.Fatal("the author and the owner of the wall may delete")
	}
	if p.CanBeDeletedBy(stranger) {
		t.Fatal("a stranger may not delete")
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

func TestNewRepost(t *testing.T) {
	t.Parallel()

	original := uuid.New()

	c, err := post.NewRepost(original, "")
	if err != nil || c.RepostOf() != original || c.Text() != "" {
		t.Fatalf("a repost without a word = %+v, %v", c, err)
	}

	if _, err = post.NewRepost(uuid.Nil, "смотрите"); !errors.Is(err, post.ErrInvalidRepost) {
		t.Fatalf("a repost of nothing: %v, want ErrInvalidRepost", err)
	}

	if plain := textOnly(t, "привет"); plain.RepostOf() != uuid.Nil {
		t.Fatalf("a plain post shares %v", plain.RepostOf())
	}
}

func TestPost_Shared(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	original := uuid.New()

	plain, err := post.New(ann, ann, textOnly(t, "привет"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plain.Shared() != plain.ID() {
		t.Fatal("reposting a post shares the post itself")
	}

	silent, err := post.NewRepost(original, "")
	if err != nil {
		t.Fatal(err)
	}
	passedOn, err := post.New(bob, bob, silent, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if passedOn.Shared() != original {
		t.Fatal("reposting a repost without a word shares its original")
	}

	said, err := post.NewRepost(original, "согласен")
	if err != nil {
		t.Fatal(err)
	}
	commented, err := post.New(bob, bob, said, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if commented.Shared() != commented.ID() {
		t.Fatal("a repost with a word of its own is shared itself")
	}
}
