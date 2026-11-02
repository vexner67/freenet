package fedi_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

func site(t *testing.T) fedi.Site {
	t.Helper()

	s, err := fedi.NewSite("https://freenet.example/")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return s
}

func TestNewURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"https", "https://mastodon.social/users/anya", true},
		{"spaces trimmed", "  https://a.example/x ", true},
		{"http", "http://a.example/x", false},
		{"no host", "https:///x", false},
		{"user info", "https://me@a.example/x", false},
		{"empty", "", false},
		{"javascript", "javascript:alert(1)", false},
		{"too long", "https://a.example/" + string(make([]byte, 2048)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := fedi.NewURL(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("NewURL(%q) error = %v, want ok %v", tt.in, err, tt.ok)
			}
			if err != nil && !errors.Is(err, fedi.ErrInvalidURL) {
				t.Fatalf("error = %v, want ErrInvalidURL", err)
			}
		})
	}
}

func TestNewSite(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"", ":80", "ftp://a.example", "https://a.example/path", "https://"} {
		if _, err := fedi.NewSite(bad); err == nil {
			t.Errorf("NewSite(%q) succeeded", bad)
		}
	}

	s := site(t)
	if s.Domain() != "freenet.example" {
		t.Fatalf("Domain = %q", s.Domain())
	}
}

func TestSiteAddresses(t *testing.T) {
	t.Parallel()

	s := site(t)
	id := uuid.MustParse("01900000-0000-7000-8000-000000000001")

	if got := s.Actor(id); got != "https://freenet.example/ap/users/01900000-0000-7000-8000-000000000001" {
		t.Fatalf("Actor = %q", got)
	}
	if got := s.Key(id); got != s.Actor(id)+"#main-key" {
		t.Fatalf("Key = %q", got)
	}

	if got, ok := s.UserOf(s.Actor(id)); !ok || got != id {
		t.Fatalf("UserOf(own actor) = %v, %v", got, ok)
	}
	if _, ok := s.UserOf("https://other.example/ap/users/" + fedi.URL(id.String())); ok {
		t.Fatal("UserOf took an actor of another server")
	}
	if _, ok := s.UserOf(s.Actor(id) + "/inbox"); ok {
		t.Fatal("UserOf took an inbox")
	}

	at := time.UnixMilli(1700000000000)
	if s.Activity(id, fedi.TypeCreate, at) == s.Activity(id, fedi.TypeUpdate, at) {
		t.Fatal("two kinds of activity share an ID")
	}
}

func TestSiteNickname(t *testing.T) {
	t.Parallel()

	s := site(t)

	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"acct:anya@freenet.example", "anya", true},
		{"anya@FREENET.example", "anya", true},
		{"@anya@freenet.example", "anya", true},
		{"https://freenet.example/anya", "anya", true},
		{"acct:anya@other.example", "", false},
		{"acct:@freenet.example", "", false},
		{"https://freenet.example/anya/photos", "", false},
		{"anya", "", false},
	}

	for _, tt := range tests {
		got, ok := s.Nickname(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Nickname(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNewRemote(t *testing.T) {
	t.Parallel()

	now := time.Now()
	from := fedi.URL("https://m.example/users/anya")
	good := fedi.RemoteDoc{
		ID:          "https://m.example/users/anya",
		Inbox:       "https://m.example/users/anya/inbox",
		SharedInbox: "https://m.example/inbox",
		KeyID:       "https://m.example/users/anya#main-key",
		PublicKey:   "-----BEGIN PUBLIC KEY-----",
	}

	r, err := fedi.NewRemote(from, good, now)
	if err != nil {
		t.Fatalf("NewRemote: %v", err)
	}
	if r.DeliveryInbox() != "https://m.example/inbox" {
		t.Fatalf("DeliveryInbox = %q, want the shared inbox", r.DeliveryInbox())
	}

	tests := []struct {
		name   string
		change func(d *fedi.RemoteDoc)
	}{
		{"another actor", func(d *fedi.RemoteDoc) { d.ID = "https://m.example/users/boris" }},
		{"inbox elsewhere", func(d *fedi.RemoteDoc) { d.Inbox = "https://evil.example/inbox" }},
		{"key elsewhere", func(d *fedi.RemoteDoc) { d.KeyID = "https://evil.example/key" }},
		{"no key", func(d *fedi.RemoteDoc) { d.PublicKey = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := good
			tt.change(&doc)

			if _, err := fedi.NewRemote(from, doc, now); !errors.Is(err, fedi.ErrInvalidActivity) {
				t.Fatalf("error = %v, want ErrInvalidActivity", err)
			}
		})
	}

	doc := good
	doc.SharedInbox = "https://evil.example/inbox"

	r, err = fedi.NewRemote(from, doc, now)
	if err != nil {
		t.Fatalf("NewRemote: %v", err)
	}
	if r.DeliveryInbox() != "https://m.example/users/anya/inbox" {
		t.Fatalf("DeliveryInbox = %q, want the own inbox when the shared one is elsewhere", r.DeliveryInbox())
	}

	if !r.Stale(now.Add(25*time.Hour), 24*time.Hour) || r.Stale(now.Add(time.Hour), 24*time.Hour) {
		t.Fatal("Stale is wrong")
	}
}

func TestPostFederates(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	own := fedi.Post{AuthorID: author, WallID: author}

	tests := []struct {
		name string
		post fedi.Post
		want bool
	}{
		{"own wall", own, true},
		{"another wall", fedi.Post{AuthorID: author, WallID: uuid.New()}, false},
		{"for a community", fedi.Post{AuthorID: author, WallID: author, FromWall: true}, false},
		{"repost", fedi.Post{AuthorID: author, WallID: author, Repost: true}, false},
		{"paid", fedi.Post{AuthorID: author, WallID: author, Paid: true}, false},
		{"archived", fedi.Post{AuthorID: author, WallID: author, Archived: true}, false},
	}

	for _, tt := range tests {
		if got := tt.post.Federates(); got != tt.want {
			t.Errorf("%s: Federates = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPostNote(t *testing.T) {
	t.Parallel()

	s := site(t)
	author, photo := uuid.New(), uuid.New()
	p := fedi.Post{ID: uuid.New(), AuthorID: author, WallID: author, Text: "hi", PhotoIDs: []uuid.UUID{photo}, NSFW: true}

	n := p.Note(s)
	if n.ID != s.Note(p.ID) || n.AttributedTo != s.Actor(author) || n.Page != s.PostPage(p.ID) {
		t.Fatalf("Note addresses = %+v", n)
	}
	if len(n.Attachments) != 1 || n.Attachments[0] != s.Photo(author, photo) {
		t.Fatalf("Attachments = %v", n.Attachments)
	}
	if !n.Sensitive || n.To[0] != fedi.Public || n.CC[0] != s.Followers(author).String() {
		t.Fatalf("Note audience or sensitivity = %+v", n)
	}
}

func TestTextHTML(t *testing.T) {
	t.Parallel()

	s := site(t)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "привет", "<p>привет</p>"},
		{"escaped", `<b>"x" & y</b>`, "<p>&lt;b&gt;&#34;x&#34; &amp; y&lt;/b&gt;</p>"},
		{"lines and paragraphs", "a\nb\n\n\nc", "<p>a<br>b</p><p>c</p>"},
		{"empty", " \n\n ", ""},
		{
			"link without the end of the sentence",
			"see https://a.example/x?y=1&z=2.",
			`<p>see <a href="https://a.example/x?y=1&amp;z=2" rel="nofollow noopener noreferrer" target="_blank">https://a.example/x?y=1&amp;z=2</a>.</p>`,
		},
		{
			"link before a guillemet",
			"«https://a.example»",
			`<p>«<a href="https://a.example" rel="nofollow noopener noreferrer" target="_blank">https://a.example</a>»</p>`,
		},
		{
			"mention",
			"hi [anya|Аня <3]!",
			`<p>hi <a href="https://freenet.example/anya">Аня &lt;3</a>!</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := fedi.TextHTML(tt.in, s); got != tt.want {
				t.Fatalf("TextHTML(%q) =\n%s\nwant\n%s", tt.in, got, tt.want)
			}
		})
	}
}

func TestAccount(t *testing.T) {
	t.Parallel()

	if _, err := fedi.NewAccount(uuid.New(), fedi.KeyPair{}, time.Now()); !errors.Is(err, fedi.ErrNoKeys) {
		t.Fatalf("NewAccount without keys: %v", err)
	}

	a, err := fedi.NewAccount(uuid.New(), fedi.KeyPair{Public: "pub", Private: "priv"}, time.Now())
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}

	closed := a.Closed()
	if !a.Open() || closed.Open() || !closed.Opened().Open() || closed.Keys() != a.Keys() {
		t.Fatal("opening and closing an account lost its state or keys")
	}
}

func TestIncoming(t *testing.T) {
	t.Parallel()

	if _, err := fedi.NewIncoming(fedi.TypeFollow, "x", "http://a.example/u", "y", nil); !errors.Is(err, fedi.ErrInvalidActivity) {
		t.Fatalf("an actor over http: %v", err)
	}

	a, err := fedi.NewIncoming(fedi.TypeDelete, "x", "https://a.example/u", "https://a.example/u#delete", nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}
	if !a.DeletesActor() {
		t.Fatal("a Delete of the actor itself does not delete it")
	}

	a.Object = "https://a.example/notes/1"
	if a.DeletesActor() {
		t.Fatal("a Delete of a note deletes the actor")
	}
}
