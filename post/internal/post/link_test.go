package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewLink(t *testing.T) {
	t.Parallel()

	good := post.LinkDraft{URL: "https://example.com/a?b=1", Title: " Пример ", Description: "Страница.", Site: "Example"}

	with := func(change func(d *post.LinkDraft)) post.LinkDraft {
		d := good
		change(&d)
		return d
	}

	tests := []struct {
		name  string
		draft post.LinkDraft
		want  error
	}{
		{"a card", good, nil},
		{"title alone", post.LinkDraft{URL: "http://example.com", Title: "Example"}, nil},
		{"no title", with(func(d *post.LinkDraft) { d.Title = "  " }), post.ErrInvalidLink},
		{"another scheme", with(func(d *post.LinkDraft) { d.URL = "javascript:alert(1)" }), post.ErrInvalidLink},
		{"no host", with(func(d *post.LinkDraft) { d.URL = "https:///path" }), post.ErrInvalidLink},
		{"long address", with(func(d *post.LinkDraft) { d.URL = "https://example.com/" + strings.Repeat("a", 2048) }), post.ErrInvalidLink},
		{"long title", with(func(d *post.LinkDraft) { d.Title = strings.Repeat("я", 257) }), post.ErrInvalidLink},
		{"title of two lines", with(func(d *post.LinkDraft) { d.Title = "a\nb" }), post.ErrInvalidLink},
		{"long description", with(func(d *post.LinkDraft) { d.Description = strings.Repeat("я", 513) }), post.ErrInvalidLink},
		{"long site", with(func(d *post.LinkDraft) { d.Site = strings.Repeat("я", 129) }), post.ErrInvalidLink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			link, err := post.NewLink(tt.draft)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewLink = %v, want %v", err, tt.want)
			}

			if err == nil && link.Title() != strings.TrimSpace(tt.draft.Title) {
				t.Fatalf("Title = %q", link.Title())
			}
		})
	}
}

func TestContent_OwnPhotos(t *testing.T) {
	t.Parallel()

	photo, picture := uuid.New(), uuid.New()

	content, err := post.NewContent("https://example.com", post.Photos{photo})
	if err != nil {
		t.Fatal(err)
	}

	if got := content.OwnPhotos(); len(got) != 1 || got[0] != photo {
		t.Fatalf("OwnPhotos without a card = %v", got)
	}

	link, err := post.NewLink(post.LinkDraft{URL: "https://example.com", Title: "Example", PhotoID: picture})
	if err != nil {
		t.Fatal(err)
	}

	carded := content.WithLink(link)
	if got := carded.OwnPhotos(); len(got) != 2 || got[0] != photo || got[1] != picture {
		t.Fatalf("OwnPhotos with a card = %v", got)
	}
	if got := carded.Photos(); len(got) != 1 {
		t.Fatalf("the card's picture is no photo of the post: %v", got)
	}
}

func TestPost_EditedKeepsLink(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	link, err := post.NewLink(post.LinkDraft{URL: "https://example.com", Title: "Example"})
	if err != nil {
		t.Fatal(err)
	}

	content, err := post.NewContent("https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	p, err := post.New(author, author, content.WithLink(link), now)
	if err != nil {
		t.Fatal(err)
	}

	edited, err := p.Edited(author, "Смотрите: https://example.com", nil, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if edited.Content().Link() != link {
		t.Fatalf("the card after an edit = %+v, want %+v", edited.Content().Link(), link)
	}
}
