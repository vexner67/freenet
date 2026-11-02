package fedi_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

const ann = fedi.URL("https://m.example/users/ann")

func TestNewAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want fedi.Address
		err  error
	}{
		{"@ann@M.Example", fedi.Address{Username: "ann", Host: "m.example"}, nil},
		{" ann@m.example ", fedi.Address{Username: "ann", Host: "m.example"}, nil},
		{"https://m.example/users/ann#main-key", fedi.Address{URL: ann}, nil},
		{"@vexner@freenet.example", fedi.Address{}, fedi.ErrInvalidAddress},
		{"https://freenet.example/vexner", fedi.Address{}, fedi.ErrInvalidAddress},
		{"ann", fedi.Address{}, fedi.ErrInvalidAddress},
		{"@ann@localhost", fedi.Address{}, fedi.ErrInvalidAddress},
		{"http://m.example/users/ann", fedi.Address{}, fedi.ErrInvalidAddress},
		{"", fedi.Address{}, fedi.ErrInvalidAddress},
	}

	for _, tt := range tests {
		got, err := fedi.NewAddress(tt.in, site(t))
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("NewAddress(%q) = %+v, %v; want %+v, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func publicNote() fedi.NoteDoc {
	return fedi.NoteDoc{
		ID:           "https://m.example/notes/1",
		AttributedTo: ann.String(),
		URL:          "https://m.example/@ann/1",
		Text:         " hello ",
		Language:     "ru-RU",
		Public:       true,
		Published:    time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
	}
}

func TestNewRemotePost(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	p, err := fedi.NewRemotePost(publicNote(), ann, now)
	if err != nil {
		t.Fatalf("NewRemotePost: %v", err)
	}

	if p.Text() != "hello" || p.Language() != "ru" || p.Page() != "https://m.example/@ann/1" || p.Sensitive() ||
		!p.Published().Equal(publicNote().Published) {
		t.Fatalf("post = %+v", p)
	}

	// The ID sorts by the time it was published, as the posts here do.
	if sec, nsec := p.ID().Time().UnixTime(); !time.Unix(sec, nsec).Equal(publicNote().Published) || p.ID().Version() != 7 {
		t.Errorf("ID %s is not a UUIDv7 of the time published", p.ID())
	}

	if !p.Speaks([]string{"ru", "en"}) || p.Speaks([]string{"en"}) {
		t.Error("Speaks: a note in Russian")
	}

	tests := []struct {
		name  string
		edit  func(*fedi.NoteDoc)
		actor fedi.URL
		err   error
	}{
		{"of another author", func(n *fedi.NoteDoc) { n.AttributedTo = "https://m.example/users/bob" }, ann, fedi.ErrInvalidActivity},
		{"on another server", func(n *fedi.NoteDoc) { n.ID = "https://evil.example/notes/1" }, ann, fedi.ErrInvalidActivity},
		{"a reply", func(n *fedi.NoteDoc) { n.InReplyTo = "https://m.example/notes/0" }, ann, fedi.ErrNotForNews},
		{"for followers", func(n *fedi.NoteDoc) { n.Public = false }, ann, fedi.ErrNotForNews},
		{"empty", func(n *fedi.NoteDoc) { n.Text = " " }, ann, fedi.ErrNotForNews},
	}

	for _, tt := range tests {
		doc := publicNote()
		tt.edit(&doc)

		if _, err := fedi.NewRemotePost(doc, tt.actor, now); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}
}

func TestRemotePostContent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	doc := publicNote()
	doc.Text = strings.Repeat("я", 60001)
	doc.Summary = "spoilers"
	doc.URL = "javascript:alert(1)"
	doc.Published = now.Add(time.Hour)
	doc.Attachments = []fedi.AttachmentDoc{
		{URL: "https://m.example/v.mp4", MediaType: "video/mp4"},
		{URL: "https://m.example/1.png", MediaType: "image/png", Description: "one"},
		{URL: "https://m.example/2.png", MediaType: "image/png"},
		{URL: "https://m.example/3.png", MediaType: "image/png"},
		{URL: "https://m.example/4.png", MediaType: "image/png"},
		{URL: "https://m.example/5.png", MediaType: "image/png"},
	}

	p, err := fedi.NewRemotePost(doc, ann, now)
	if err != nil {
		t.Fatalf("NewRemotePost: %v", err)
	}

	if n := len([]rune(p.Text())); n != 60000 {
		t.Errorf("text of %d characters, want cut to 60000", n)
	}

	if !p.Sensitive() || p.Summary() != "spoilers" {
		t.Error("a note with a content warning is sensitive")
	}

	if p.Page() != p.URI() {
		t.Errorf("page %q, want the note itself when its url is no https address", p.Page())
	}

	if !p.Published().Equal(now) {
		t.Errorf("published %v, want a time in the future made now", p.Published())
	}

	if images := p.Images(); len(images) != 4 || images[0].URL != "https://m.example/1.png" || images[0].Description != "one" {
		t.Errorf("images %+v, want the first four pictures", images)
	}

	if videos := p.Videos(); len(videos) != 1 || videos[0].URL != "https://m.example/v.mp4" {
		t.Errorf("videos %+v, want the video", videos)
	}

	edited := publicNote()
	edited.Text = "edited"
	edited.Updated = now.Add(-time.Minute)

	e, err := p.Edited(edited, now)
	if err != nil || e.ID() != p.ID() || e.Text() != "edited" || !e.Updated().Equal(edited.Updated) {
		t.Fatalf("Edited = %+v, %v", e, err)
	}

	edited.InReplyTo = "https://m.example/notes/0"
	if _, err = p.Edited(edited, now); !errors.Is(err, fedi.ErrNotForNews) {
		t.Errorf("an edit into a reply = %v, want ErrNotForNews", err)
	}
}

func TestRemotePost_Videos(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	video := fedi.AttachmentDoc{
		URL: "https://m.example/v.mp4", MediaType: "video/mp4", Description: " cat ",
		Cover: "https://m.example/cover.jpg", Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
		Width: 1280, Height: 720, Duration: 84 * time.Second,
	}

	tests := []struct {
		name string
		edit func(d *fedi.AttachmentDoc)
		want fedi.Video
	}{
		{
			name: "everything told",
			edit: func(*fedi.AttachmentDoc) {},
			want: fedi.Video{
				URL: "https://m.example/v.mp4", Cover: "https://m.example/cover.jpg", Description: "cat",
				Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Width: 1280, Height: 720, Duration: 84 * time.Second,
			},
		},
		{
			name: "nothing sound but the video",
			edit: func(d *fedi.AttachmentDoc) {
				d.Cover, d.Blurhash, d.Width, d.Height, d.Duration = "http://m.example/c.jpg", "LEHV6nWB2yk8pyo0adR*.7kCMdn", 20000, 720, -time.Second
			},
			want: fedi.Video{URL: "https://m.example/v.mp4", Description: "cat"},
		},
		{
			name: "a blurhash of a strange character",
			edit: func(d *fedi.AttachmentDoc) { d.Blurhash = "LEHV6nWB2yk8pyo0adR*.7kCMd\\" },
			want: fedi.Video{
				URL: "https://m.example/v.mp4", Cover: "https://m.example/cover.jpg", Description: "cat",
				Width: 1280, Height: 720, Duration: 84 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := publicNote()
			doc.Text = ""
			v := video
			tt.edit(&v)
			doc.Attachments = []fedi.AttachmentDoc{v}

			p, err := fedi.NewRemotePost(doc, ann, now)
			if err != nil {
				t.Fatalf("NewRemotePost: %v, want a post of a video alone", err)
			}

			if videos := p.Videos(); len(videos) != 1 || videos[0] != tt.want {
				t.Errorf("videos %+v, want %+v", videos, tt.want)
			}
		})
	}
}

func TestFollowing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	f, err := fedi.NewFollowing(ann, true, now)
	if err != nil {
		t.Fatalf("NewFollowing: %v", err)
	}

	if f.Actor() != ann || !f.Featured() || f.Accepted() || f.FollowID() == uuid.Nil {
		t.Fatalf("following = %+v", f)
	}

	if g := f.WithAccepted(true).WithFeatured(false); !g.Accepted() || g.Featured() || g.FollowID() != f.FollowID() {
		t.Fatalf("changed = %+v", g)
	}

	s := site(t)
	if s.Follow(f.FollowID()) != "https://freenet.example/ap/actor#follow-"+fedi.URL(f.FollowID().String()) {
		t.Errorf("Follow = %s", s.Follow(f.FollowID()))
	}

	if id, ok := s.UserOf(s.Actor(uuid.Nil)); !ok || id != uuid.Nil {
		t.Error("UserOf: the actor of the site")
	}
}

func TestRemoteProfile(t *testing.T) {
	t.Parallel()

	r, err := fedi.NewRemote(ann, fedi.RemoteDoc{
		ID: ann.String(), Inbox: ann.String() + "/inbox", KeyID: ann.String() + "#main-key", PublicKey: "PEM",
		Name: strings.Repeat("a", 200), Username: "ann", Icon: "https://m.example/ann.png", Page: "https://m.example/@ann",
	}, time.Now())
	if err != nil {
		t.Fatalf("NewRemote: %v", err)
	}

	p := r.Profile()
	if len(p.Name) != 100 || p.Icon != "https://m.example/ann.png" || p.Page != "https://m.example/@ann" || r.Handle() != "@ann@m.example" {
		t.Fatalf("profile = %+v, handle %q", p, r.Handle())
	}
}
