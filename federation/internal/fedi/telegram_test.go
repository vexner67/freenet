package fedi_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
)

const meduza = fedi.URL("https://t.me/meduzalive")

func TestNewChannel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	doc := fedi.ChannelDoc{Title: " Медуза — LIVE ", Username: "MeduzaLive", Photo: "https://cdn4.telesco.pe/file/a.jpg"}

	r, err := fedi.NewChannel("meduzalive", doc, now)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}

	p := r.Profile()
	if r.ID() != meduza || !r.Channel() || r.Handle() != "@meduzalive" || r.Inbox() != "" ||
		p.Name != "Медуза — LIVE" || p.Icon != "https://cdn4.telesco.pe/file/a.jpg" || p.Page != meduza {
		t.Fatalf("channel = %+v, %+v", r, p)
	}

	if !fedi.IsChannel("https://t.me/meduzalive/1") || fedi.IsChannel(ann) || fedi.ChannelName(meduza) != "meduzalive" {
		t.Fatal("IsChannel or ChannelName are wrong")
	}

	doc.Username = "another"
	if _, err = fedi.NewChannel("meduzalive", doc, now); !errors.Is(err, fedi.ErrInvalidActivity) {
		t.Fatalf("a page of another channel: err = %v", err)
	}
}

func channelNote() fedi.NoteDoc {
	return fedi.NoteDoc{
		ID:           "https://t.me/meduzalive/7",
		AttributedTo: meduza.String(),
		URL:          "https://t.me/meduzalive/7",
		Text:         "news",
		Public:       true,
		Published:    time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		Attachments:  []fedi.AttachmentDoc{{URL: "https://cdn4.telesco.pe/file/1.jpg", MediaType: "image/jpeg"}},
	}
}

func TestRemotePostReread(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	p, err := fedi.NewRemotePost(channelNote(), meduza, now)
	if err != nil {
		t.Fatalf("NewRemotePost: %v", err)
	}

	same, changed, err := p.Reread(channelNote(), now)
	if err != nil || changed || !same.Updated().IsZero() {
		t.Fatalf("the same page: changed %v, updated %v, err %v", changed, same.Updated(), err)
	}

	moved := channelNote()
	moved.Attachments[0].URL = "https://cdn4.telesco.pe/file/2.jpg"
	again, changed, err := p.Reread(moved, now)
	if err != nil || !changed || !again.Updated().IsZero() || again.Images()[0].URL != "https://cdn4.telesco.pe/file/2.jpg" {
		t.Fatalf("a picture at another address: changed %v, updated %v, err %v", changed, again.Updated(), err)
	}

	edited := channelNote()
	edited.Text = "news, corrected"
	again, changed, err = p.Reread(edited, now)
	if err != nil || !changed || !again.Updated().Equal(now) || again.Text() != "news, corrected" {
		t.Fatalf("another text: changed %v, updated %v, err %v", changed, again.Updated(), err)
	}
}

func TestChannelDocDeleted(t *testing.T) {
	t.Parallel()

	page := func(numbers ...string) fedi.ChannelDoc {
		var doc fedi.ChannelDoc
		for _, n := range numbers {
			doc.Posts = append(doc.Posts, fedi.NoteDoc{ID: "https://t.me/meduzalive/" + n})
		}
		return doc
	}
	kept := []fedi.URL{
		"https://t.me/meduzalive/5", "https://t.me/meduzalive/10", "https://t.me/meduzalive/11",
		"https://t.me/meduzalive/12", "https://t.me/meduzalive/14", "https://m.example/notes/13",
	}

	tests := []struct {
		name string
		doc  fedi.ChannelDoc
		want []fedi.URL
	}{
		{
			name: "missing among those shown and newer than all",
			doc:  page("10", "12", "13"),
			want: []fedi.URL{"https://t.me/meduzalive/11", "https://t.me/meduzalive/14"},
		},
		{name: "all shown", doc: page("10", "11", "12", "14")},
		{name: "an empty page", doc: page()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.doc.Deleted(kept); !slices.Equal(got, tt.want) {
				t.Fatalf("Deleted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemotePostLink(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	note := channelNote()
	note.Link = fedi.LinkDoc{
		URL: "https://meduza.io/news/1", Site: " Meduza ", Title: " Новость ", Description: "Что случилось",
		Image: "https://cdn4.telesco.pe/file/card.jpg",
	}

	p, err := fedi.NewRemotePost(note, meduza, now)
	if err != nil {
		t.Fatalf("NewRemotePost: %v", err)
	}

	want := fedi.Link{
		URL: "https://meduza.io/news/1", Site: "Meduza", Title: "Новость", Description: "Что случилось",
		Image: "https://cdn4.telesco.pe/file/card.jpg",
	}
	if p.Link() != want {
		t.Fatalf("Link = %+v, want %+v", p.Link(), want)
	}

	for name, l := range map[string]fedi.LinkDoc{
		"no title":       {URL: "https://meduza.io/news/1"},
		"not an address": {URL: "javascript:alert(1)", Title: "x"},
		"not https":      {URL: "http://meduza.io/news/1", Title: "x"},
	} {
		note.Link = l
		if p, err = fedi.NewRemotePost(note, meduza, now); err != nil || p.Link() != (fedi.Link{}) {
			t.Fatalf("%s: Link = %+v, err %v", name, p.Link(), err)
		}
	}

	note.Link = fedi.LinkDoc{}
	if _, changed, _ := p.WithLink(want).Reread(note, now); !changed {
		t.Fatal("a card gone is no change")
	}
}
