package tgpage_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/tgpage"
)

func TestParse(t *testing.T) {
	t.Parallel()

	page, err := os.ReadFile("testdata/channel.html")
	if err != nil {
		t.Fatal(err)
	}

	doc, err := tgpage.Parse(page, "news_room")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if doc.Title != "Новости & люди" || doc.Username != "News_Room" || doc.Photo != "https://cdn4.telesco.pe/file/photo.jpg" {
		t.Fatalf("channel = %q %q %q", doc.Title, doc.Username, doc.Photo)
	}

	if len(doc.Posts) != 2 {
		t.Fatalf("posts = %d, want 2: one with no date is left out", len(doc.Posts))
	}

	first := doc.Posts[0]
	if first.ID != "https://t.me/news_room/101" || first.AttributedTo != "https://t.me/news_room" || !first.Public {
		t.Errorf("first = %q by %q, public %v", first.ID, first.AttributedTo, first.Public)
	}
	if want := time.Date(2026, 11, 2, 16, 1, 8, 0, time.UTC); !first.Published.Equal(want) {
		t.Errorf("published = %v, want %v", first.Published, want)
	}
	if want := "Главное за день (https://bit.ly/x) https://www.meduza.io/x @meduzalive #news\n\nВторая строка 🔥"; first.Text != want {
		t.Errorf("text = %q, want %q", first.Text, want)
	}

	card := fedi.LinkDoc{
		URL: "https://example.com/news", Site: "Example", Title: "Новость дня", Description: "Что случилось",
		Image: "https://cdn4.telesco.pe/file/preview.jpg",
	}
	if first.Link != card {
		t.Errorf("link = %+v, want %+v", first.Link, card)
	}
	if doc.Posts[1].Link != (fedi.LinkDoc{}) {
		t.Errorf("a post with no card has %+v", doc.Posts[1].Link)
	}

	photos := []fedi.AttachmentDoc{
		{URL: "https://cdn4.telesco.pe/file/one.jpg", MediaType: "image/jpeg"},
		{URL: "https://cdn4.telesco.pe/file/two.jpg", MediaType: "image/jpeg"},
	}
	if len(first.Attachments) != 2 || first.Attachments[0] != photos[0] || first.Attachments[1] != photos[1] {
		t.Errorf("attachments = %+v, want the two photos and not the preview", first.Attachments)
	}

	videos := doc.Posts[1].Attachments
	if len(videos) != 2 {
		t.Fatalf("videos = %+v", videos)
	}

	want := fedi.AttachmentDoc{
		URL: "https://cdn4.telesco.pe/file/clip.mp4?token=abc", MediaType: "video/mp4", Cover: "https://cdn4.telesco.pe/file/thumb.jpg",
		Width: 848, Height: 477, Duration: time.Hour + 2*time.Minute + 3*time.Second,
	}
	if videos[0] != want {
		t.Errorf("video = %+v, want %+v", videos[0], want)
	}

	big := fedi.AttachmentDoc{
		URL: "https://t.me/news_room/102", MediaType: "video/mp4", Cover: "https://cdn4.telesco.pe/file/big.jpg", Duration: 42 * time.Second,
	}
	if videos[1] != big {
		t.Errorf("a video too big for the page = %+v, want %+v", videos[1], big)
	}
}

func TestParseNoChannel(t *testing.T) {
	t.Parallel()

	_, err := tgpage.Parse([]byte(`<html><body><div class="tgme_page">A person</div></body></html>`), "anya")
	if !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("err = %v, want fedi.ErrNotFound", err)
	}
}
