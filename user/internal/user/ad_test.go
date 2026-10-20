package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewAdURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want user.AdURL
		err  error
	}{
		{name: "https", in: " https://example.com/a?b=c ", want: "https://example.com/a?b=c"},
		{name: "http", in: "http://example.com", want: "http://example.com"},
		{name: "no scheme", in: "example.com/shop", want: "https://example.com/shop"},
		{name: "empty", in: "  ", err: user.ErrAdURLInvalid},
		{name: "another scheme", in: "javascript://alert(1)", err: user.ErrAdURLInvalid},
		{name: "user in it", in: "https://ann@example.com", err: user.ErrAdURLInvalid},
		{name: "spaces", in: "https://example.com/a b", err: user.ErrAdURLInvalid},
		{name: "too long", in: "https://example.com/" + strings.Repeat("a", user.MaxAdURLLength), err: user.ErrAdURLTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewAdURL(tt.in)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewAdURL(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestNewAdTitleAndText(t *testing.T) {
	t.Parallel()

	if got, err := user.NewAdTitle("  Скидки   на\nвсё "); err != nil || got != "Скидки на всё" {
		t.Fatalf("NewAdTitle = %q, %v", got, err)
	}
	if _, err := user.NewAdTitle(" ㅤ "); !errors.Is(err, user.ErrInvisibleText) {
		t.Fatalf("an invisible title: %v", err)
	}
	if _, err := user.NewAdTitle("  "); !errors.Is(err, user.ErrAdTitleEmpty) {
		t.Fatalf("an empty title: %v", err)
	}
	if _, err := user.NewAdTitle(strings.Repeat("ё", user.MaxAdTitleLength+1)); !errors.Is(err, user.ErrAdTitleTooLong) {
		t.Fatalf("a long title: %v", err)
	}

	if got, err := user.NewAdText(" две\nстроки "); err != nil || got != "две\nстроки" {
		t.Fatalf("NewAdText = %q, %v", got, err)
	}
	if got, err := user.NewAdText(""); err != nil || got != "" {
		t.Fatalf("an empty text: %q, %v", got, err)
	}
	if _, err := user.NewAdText(strings.Repeat("ё", user.MaxAdTextLength+1)); !errors.Is(err, user.ErrAdTextTooLong) {
		t.Fatalf("a long text: %v", err)
	}
}

func TestAd_Edited(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	cover := user.AdPhoto{OwnerID: owner, PhotoID: uuid.New()}

	if _, err := user.NewAd(user.AdDraft{URL: "https://a.example", Title: "A", Photo: user.AdPhoto{PhotoID: uuid.New()}}); !errors.Is(err, user.ErrAdPhotoNoOwner) {
		t.Fatalf("a cover without its owner: %v", err)
	}

	ad, err := user.NewAd(user.AdDraft{URL: "https://a.example", Title: "A", Text: "a", Photo: cover, Active: true})
	if err != nil {
		t.Fatal(err)
	}

	hidden := false
	title := user.AdTitle("B")
	edited, dropped, err := ad.Edited(user.AdChange{Title: &title, Active: &hidden})
	if err != nil || edited.Title() != "B" || edited.IsActive() || edited.Text() != "a" || edited.Photo() != cover || !dropped.IsZero() {
		t.Fatalf("Edited = %+v, %+v, %v", edited, dropped, err)
	}
	if ad.Title() != "A" || !ad.IsActive() {
		t.Fatal("Edited changed the ad it was called on")
	}

	// The cover the ad has, named by another administrator, is kept.
	same := user.AdPhoto{OwnerID: uuid.New(), PhotoID: cover.PhotoID}
	if edited, dropped, err = ad.Edited(user.AdChange{Photo: &same}); err != nil || edited.Photo() != cover || !dropped.IsZero() {
		t.Fatalf("the same cover: %+v, %+v, %v", edited.Photo(), dropped, err)
	}

	another := user.AdPhoto{OwnerID: uuid.New(), PhotoID: uuid.New()}
	if edited, dropped, err = ad.Edited(user.AdChange{Photo: &another}); err != nil || edited.Photo() != another || dropped != cover {
		t.Fatalf("another cover: %+v, %+v, %v", edited.Photo(), dropped, err)
	}

	none := user.AdPhoto{}
	if edited, dropped, err = ad.Edited(user.AdChange{Photo: &none}); err != nil || !edited.Photo().IsZero() || dropped != cover {
		t.Fatalf("no cover: %+v, %+v, %v", edited.Photo(), dropped, err)
	}
}
