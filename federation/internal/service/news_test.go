package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
)

// note is a public note of anya, in language.
func note(id, language string, published time.Time) fedi.NoteDoc {
	return fedi.NoteDoc{
		ID:           "https://m.example/notes/" + id,
		AttributedTo: remoteActor.String(),
		Text:         "note " + id,
		Language:     language,
		Public:       true,
		Published:    published,
		Attachments:  []fedi.AttachmentDoc{{URL: "https://m.example/" + id + ".png", MediaType: "image/png"}},
	}
}

// send makes anya send an activity about doc, signed with her key.
func (f *fixture) send(t *testing.T, typ string, doc fedi.NoteDoc) {
	t.Helper()

	a, err := fedi.NewIncoming(typ, doc.ID+"#"+typ, remoteActor.String(), doc.ID, nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	if typ != fedi.TypeDelete {
		a.Note = &doc
	}

	if err = f.svc.Receive(context.Background(), signedBy("anya-key"), a, uuid.Nil); err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
}

// accept makes anya's server accept the follow of the site it was sent.
func (f *fixture) accept(t *testing.T) {
	t.Helper()

	follow := f.sender.sent[len(f.sender.sent)-1].activity
	if follow.Type != fedi.TypeFollow {
		t.Fatalf("last sent %s, want Follow", follow.Type)
	}

	a, err := fedi.NewIncoming(fedi.TypeAccept, "https://m.example/a/1", remoteActor.String(), follow.ID.String(), nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	if err = f.svc.Receive(context.Background(), signedBy("anya-key"), a, uuid.Nil); err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

func (f *fixture) newsTexts(t *testing.T, q service.NewsQuery) []string {
	t.Helper()

	if q.Limit == 0 {
		q.Limit = 20
	}

	posts, err := f.svc.News(context.Background(), q)
	if err != nil {
		t.Fatalf("News: %v", err)
	}

	texts := make([]string, len(posts))
	for i, p := range posts {
		texts[i] = p.Post.Text()
	}

	return texts
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestFeaturedNews(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	a, err := f.svc.Feature(ctx, "@anya@m.example")
	if err != nil {
		t.Fatalf("Feature: %v", err)
	}

	if a.Remote.ID() != remoteActor || a.Accepted {
		t.Fatalf("featured %+v, want anya not yet accepted", a)
	}

	follow := f.sender.sent[0]
	if len(f.sender.sent) != 1 || follow.inbox != remoteActor+"/inbox" || follow.signerID != uuid.Nil ||
		follow.activity.Actor != f.site.Actor(uuid.Nil) || follow.activity.Person != remoteActor {
		t.Fatalf("sent %+v, want a Follow of the site to anya", f.sender.sent)
	}

	// Featured again: still one follow.
	if _, err = f.svc.Feature(ctx, "anya@m.example"); err != nil || len(f.sender.sent) != 1 {
		t.Fatalf("Feature again: %v, sent %d", err, len(f.sender.sent))
	}

	f.accept(t)

	featured, err := f.svc.Featured(ctx)
	if err != nil || len(featured) != 1 || !featured[0].Accepted {
		t.Fatalf("Featured = %+v, %v; want anya accepted", featured, err)
	}

	at := f.now.Add(-time.Hour)
	f.send(t, fedi.TypeCreate, note("1", "en", at))
	f.send(t, fedi.TypeCreate, note("2", "de", at.Add(time.Minute)))
	f.send(t, fedi.TypeCreate, note("3", "", at.Add(2*time.Minute)))
	reply := note("4", "ru", at.Add(3*time.Minute))
	reply.InReplyTo = "https://m.example/notes/1"
	f.send(t, fedi.TypeCreate, reply)
	followers := note("5", "ru", at.Add(4*time.Minute))
	followers.Public = false
	f.send(t, fedi.TypeCreate, followers)

	if got := f.newsTexts(t, service.NewsQuery{Featured: true}); !equal(got, []string{"note 3", "note 1"}) {
		t.Fatalf("featured news = %q, want the notes in Russian, English or no language, newest first", got)
	}

	if got := f.newsTexts(t, service.NewsQuery{Reader: uuid.New()}); len(got) != 0 {
		t.Fatalf("the news of someone reading nobody = %q", got)
	}

	edited := note("1", "en", at)
	edited.Text = "edited"
	f.send(t, fedi.TypeUpdate, edited)
	f.send(t, fedi.TypeDelete, note("3", "", at))

	if got := f.newsTexts(t, service.NewsQuery{Featured: true}); !equal(got, []string{"edited"}) {
		t.Fatalf("featured news after an edit and a delete = %q", got)
	}

	if err = f.svc.Unfeature(ctx, remoteActor); err != nil {
		t.Fatalf("Unfeature: %v", err)
	}

	undo := f.sender.sent[len(f.sender.sent)-1].activity
	if undo.Type != fedi.TypeUndo || undo.Follow == nil || undo.Follow.ID != follow.activity.ID.String() {
		t.Fatalf("sent %+v, want an Undo of the follow", undo)
	}

	if got := f.newsTexts(t, service.NewsQuery{Featured: true}); len(got) != 0 {
		t.Fatalf("news after Unfeature = %q, want none", got)
	}
}

func TestNotesOfOthers(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	// Nobody here follows anya: her notes are dropped unread.
	f.send(t, fedi.TypeCreate, note("1", "en", f.now))

	if got := f.newsTexts(t, service.NewsQuery{Featured: true}); len(got) != 0 {
		t.Fatalf("news = %q, want none", got)
	}

	if _, err := f.svc.Feature(context.Background(), "@anya@m.example"); err != nil {
		t.Fatalf("Feature: %v", err)
	}

	// A note of hers on another server, or of another author, is no
	// note of hers.
	forged := note("2", "en", f.now)
	forged.ID = "https://evil.example/notes/2"
	a, err := fedi.NewIncoming(fedi.TypeCreate, "https://m.example/c/2", remoteActor.String(), forged.ID, nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}
	a.Note = &forged

	if err = f.svc.Receive(context.Background(), signedBy("anya-key"), a, uuid.Nil); !errors.Is(err, fedi.ErrInvalidActivity) {
		t.Fatalf("a note of another server = %v, want ErrInvalidActivity", err)
	}

	// Signed by someone else.
	doc := note("3", "en", f.now)
	a.ID, a.Object, a.Note = doc.ID, doc.ID, &doc

	if err = f.svc.Receive(context.Background(), signedBy("forged"), a, uuid.Nil); !errors.Is(err, fedi.ErrBadSignature) {
		t.Fatalf("a forged note = %v, want ErrBadSignature", err)
	}
}

func TestSubscriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	reader, other := uuid.New(), uuid.New()

	if _, err := f.svc.Subscribe(ctx, reader, "https://m.example/users/anya"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	f.accept(t)
	f.send(t, fedi.TypeCreate, note("1", "de", f.now))

	if got := f.newsTexts(t, service.NewsQuery{Reader: reader}); !equal(got, []string{"note 1"}) {
		t.Fatalf("the news of the reader = %q, want her note in any language", got)
	}

	for name, q := range map[string]service.NewsQuery{
		"another person": {Reader: other},
		"everyone":       {Featured: true},
		"sensitive":      {Reader: reader, HideSensitive: true},
	} {
		if name == "sensitive" {
			cw := note("2", "en", f.now)
			cw.Summary = "spoilers"
			f.send(t, fedi.TypeCreate, cw)
		}

		for _, text := range f.newsTexts(t, q) {
			if name != "sensitive" || text == "note 2" {
				t.Errorf("%s: news has %q", name, text)
			}
		}
	}

	a, subscribed, err := f.svc.Find(ctx, reader, "@anya@m.example")
	if err != nil || !subscribed || !a.Accepted {
		t.Fatalf("Find = %+v %v %v, want anya read and accepted", a, subscribed, err)
	}

	subs, err := f.svc.Subscriptions(ctx, reader)
	if err != nil || len(subs) != 1 || subs[0].Remote.ID() != remoteActor {
		t.Fatalf("Subscriptions = %+v, %v", subs, err)
	}

	// The other reader keeps the follow when the first one leaves.
	if _, err = f.svc.Subscribe(ctx, other, "@anya@m.example"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	sent := len(f.sender.sent)

	if err = f.svc.Unsubscribe(ctx, reader, remoteActor); err != nil || len(f.sender.sent) != sent {
		t.Fatalf("Unsubscribe of one of two: %v, sent %d", err, len(f.sender.sent)-sent)
	}

	if err = f.svc.Unsubscribe(ctx, other, remoteActor); err != nil || len(f.sender.sent) != sent+1 {
		t.Fatalf("Unsubscribe of the last: %v, sent %d, want an Undo", err, len(f.sender.sent)-sent)
	}

	if got := f.newsTexts(t, service.NewsQuery{Reader: other}); len(got) != 0 {
		t.Fatalf("news after the last unsubscribed = %q", got)
	}
}

func TestLookupErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	tests := []struct {
		address string
		want    error
	}{
		{"anya", fedi.ErrInvalidAddress},
		{"@vexner@freenet.example", fedi.ErrInvalidAddress},
		{"http://m.example/users/anya", fedi.ErrInvalidAddress},
		{"@nobody@m.example", fedi.ErrAccountNotFound},
		{"https://m.example/users/nobody", fedi.ErrAccountNotFound},
	}

	for _, tt := range tests {
		if _, err := f.svc.Lookup(context.Background(), tt.address); !errors.Is(err, tt.want) {
			t.Errorf("Lookup(%q) = %v, want %v", tt.address, err, tt.want)
		}
	}
}

func TestTooManySubscriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	reader := uuid.New()

	for i := range service.MaxSubscriptions {
		if err := f.subscriptions.Add(ctx, reader, fedi.URL("https://m.example/users/"+string(rune('a'+i%26))+uuid.NewString()), f.now); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	if _, err := f.svc.Subscribe(ctx, reader, "@anya@m.example"); !errors.Is(err, fedi.ErrTooManySubscriptions) {
		t.Fatalf("Subscribe over the limit = %v", err)
	}
}

func TestLookupLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.limiter.left = 0

	if _, _, err := f.svc.Find(context.Background(), uuid.New(), "@anya@m.example"); !errors.Is(err, fedi.ErrTooManyLookups) {
		t.Fatalf("Find over the limit = %v", err)
	}

	if _, err := f.svc.Subscribe(context.Background(), uuid.New(), "@anya@m.example"); !errors.Is(err, fedi.ErrTooManyLookups) {
		t.Fatalf("Subscribe over the limit = %v", err)
	}
}

func TestVideoCover(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.svc.Subscribe(ctx, uuid.New(), "@anya@m.example"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	doc := note("1", "en", f.now)
	doc.Attachments = []fedi.AttachmentDoc{
		{URL: "https://m.example/covered.mp4", MediaType: "video/mp4", Cover: "https://m.example/cover.jpg"},
		{URL: "https://m.example/plain.mp4", MediaType: "video/mp4"},
		{URL: "https://m.example/broken.mp4", MediaType: "video/mp4"},
	}
	f.send(t, fedi.TypeCreate, doc)
	f.frames.frames["https://m.example/plain.mp4"] = []byte("jpeg")
	f.frames.fail = errors.New("ffmpeg failed")

	p, err := f.news.ByURI(ctx, "https://m.example/notes/1")
	if err != nil {
		t.Fatalf("ByURI: %v", err)
	}

	if m, err := f.svc.VideoCover(ctx, p.ID(), 0); err != nil || m.ContentType != "image/png" || f.media.fetched != "https://m.example/cover.jpg" {
		t.Fatalf("cover the author chose = %+v, %v; fetched %s", m, err, f.media.fetched)
	}

	// A failure is no answer: the next read cuts again.
	if _, err := f.svc.VideoCover(ctx, p.ID(), 1); !errors.Is(err, f.frames.fail) {
		t.Fatalf("a failed cut = %v, want its error", err)
	}
	f.frames.fail, f.frames.cuts = nil, 0

	for range 2 {
		if m, err := f.svc.VideoCover(ctx, p.ID(), 1); err != nil || string(m.Body) != "jpeg" || m.ContentType != "image/jpeg" {
			t.Fatalf("first frame = %+v, %v", m, err)
		}

		if _, err := f.svc.VideoCover(ctx, p.ID(), 2); !errors.Is(err, fedi.ErrNotFound) {
			t.Fatalf("a video with no frame = %v, want ErrNotFound", err)
		}
	}

	if f.frames.cuts != 2 {
		t.Errorf("%d cuts, want each video cut once", f.frames.cuts)
	}

	for _, index := range []int{-1, 3} {
		if _, err := f.svc.VideoCover(ctx, p.ID(), index); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("VideoCover %d = %v, want ErrNotFound", index, err)
		}
	}

	if _, err := f.svc.VideoCover(ctx, uuid.New(), 0); !errors.Is(err, fedi.ErrNotFound) {
		t.Errorf("a cover of no post = %v, want ErrNotFound", err)
	}
}

func TestPictures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.svc.Subscribe(ctx, uuid.New(), "@anya@m.example"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	f.send(t, fedi.TypeCreate, note("1", "en", f.now))

	posts, err := f.svc.News(ctx, service.NewsQuery{Featured: false, Reader: uuid.Nil, Limit: 1})
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("news of nobody = %d posts", len(posts))
	}

	p, err := f.news.ByURI(ctx, "https://m.example/notes/1")
	if err != nil {
		t.Fatalf("ByURI: %v", err)
	}

	if m, err := f.svc.Picture(ctx, p.ID(), 0); err != nil || m.ContentType != "image/png" || f.media.fetched != "https://m.example/1.png" {
		t.Fatalf("Picture = %+v, %v; fetched %s", m, err, f.media.fetched)
	}

	for _, index := range []int{-1, 1} {
		if _, err := f.svc.Picture(ctx, p.ID(), index); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("Picture %d = %v, want ErrNotFound", index, err)
		}
	}

	// anya has no photo; a stranger is no account the site knows.
	for _, actor := range []fedi.URL{remoteActor, "https://m.example/users/stranger"} {
		if _, err := f.svc.Avatar(ctx, actor); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("Avatar of %s = %v, want ErrNotFound", actor, err)
		}
	}
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	reader := uuid.New()

	if _, err := f.svc.Subscribe(ctx, reader, "@anya@m.example"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	f.send(t, fedi.TypeCreate, note("old", "en", f.now.Add(-100*24*time.Hour)))
	f.send(t, fedi.TypeCreate, note("new", "en", f.now.Add(-time.Hour)))

	if n, err := f.svc.Cleanup(ctx); err != nil || n != 1 {
		t.Fatalf("Cleanup = %d, %v; want the old note", n, err)
	}

	if got := f.newsTexts(t, service.NewsQuery{Reader: reader}); !equal(got, []string{"note new"}) {
		t.Fatalf("news after Cleanup = %q", got)
	}
}

func TestSiteActor(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	p, err := f.svc.Finger(context.Background(), "acct:freenet.example@freenet.example")
	if err != nil {
		t.Fatalf("Finger: %v", err)
	}

	if p.ID != f.site.Actor(uuid.Nil) || !p.Service || p.PublicKey != "our-public" || p.Nickname != "freenet.example" {
		t.Fatalf("site actor = %+v", p)
	}
}
