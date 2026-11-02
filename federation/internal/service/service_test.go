package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/inmem"
	"github.com/vexner67/freenet/federation/internal/service"
)

const remoteActor = fedi.URL("https://m.example/users/anya")

type people map[uuid.UUID]service.Person

func (p people) ByID(_ context.Context, id uuid.UUID) (service.Person, error) {
	person, ok := p[id]
	if !ok {
		return service.Person{}, fedi.ErrNotFound
	}

	return person, nil
}

func (p people) ByNickname(_ context.Context, nickname string) (service.Person, error) {
	for _, person := range p {
		if person.Nickname == nickname {
			return person, nil
		}
	}

	return service.Person{}, fedi.ErrNotFound
}

type posts map[uuid.UUID]fedi.Post

func (p posts) Post(_ context.Context, id uuid.UUID) (fedi.Post, error) {
	post, ok := p[id]
	if !ok {
		return fedi.Post{}, fedi.ErrNotFound
	}

	return post, nil
}

// resolver serves the documents of remote actors; one in gone is
// deleted for good.
type resolver struct {
	docs  map[fedi.URL]fedi.RemoteDoc
	gone  map[fedi.URL]bool
	reads int
}

func (r *resolver) Actor(_ context.Context, id fedi.URL, _ *fedi.Account) (fedi.RemoteDoc, error) {
	r.reads++

	if r.gone[id] {
		return fedi.RemoteDoc{}, fedi.ErrGone
	}

	doc, ok := r.docs[id]
	if !ok {
		return fedi.RemoteDoc{}, errors.New("unreachable")
	}

	return doc, nil
}

// signatures takes a request as signed with key when its signature
// header is "keyID key".
type signatures struct{}

func (signatures) KeyID(r service.SignedRequest) (fedi.URL, error) {
	id, _, _ := cut(r.Headers["signature"])
	if id == "" {
		return "", fedi.ErrBadSignature
	}

	return fedi.URL(id), nil
}

func (signatures) Verify(r service.SignedRequest, publicKey string, _ time.Time) error {
	if _, key, _ := cut(r.Headers["signature"]); key != publicKey {
		return fedi.ErrBadSignature
	}

	return nil
}

func cut(s string) (string, string, bool) {
	for i := range s {
		if s[i] == ' ' {
			return s[:i], s[i+1:], true
		}
	}

	return s, "", false
}

type sent struct {
	inbox    fedi.URL
	signerID uuid.UUID
	activity fedi.Outgoing
}

type sender struct {
	sent []sent
}

func (s *sender) Send(_ context.Context, inbox fedi.URL, signerID uuid.UUID, a fedi.Outgoing) error {
	s.sent = append(s.sent, sent{inbox: inbox, signerID: signerID, activity: a})
	return nil
}

type keys struct{}

func (keys) NewKeys() (fedi.KeyPair, error) {
	return fedi.KeyPair{Public: "our-public", Private: "our-private"}, nil
}

type limiter struct {
	left int
}

func (l *limiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
	l.left--
	return l.left >= 0, nil
}

// finder knows the actors of addresses username@host.
type finder map[string]fedi.URL

func (f finder) Finger(_ context.Context, username, host string) (fedi.URL, error) {
	id, ok := f[username+"@"+host]
	if !ok {
		return "", fedi.ErrNotFound
	}

	return id, nil
}

// media serves any address as a picture and remembers the last one.
type media struct {
	fetched fedi.URL
}

func (m *media) Fetch(_ context.Context, u fedi.URL) (service.Media, error) {
	m.fetched = u
	return service.Media{Body: []byte("png"), ContentType: "image/png"}, nil
}

// frames cut frames of the videos it has, counting the cuts.
type frames struct {
	frames map[fedi.URL][]byte
	cuts   int
	fail   error
}

func (f *frames) Frame(_ context.Context, video fedi.URL) ([]byte, error) {
	f.cuts++
	if f.fail != nil {
		return nil, f.fail
	}

	frame, ok := f.frames[video]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return frame, nil
}

type fixture struct {
	svc       *service.Service
	site      fedi.Site
	user      uuid.UUID
	people    people
	posts     posts
	resolver  *resolver
	sender    *sender
	followers *inmem.FollowerRepository
	accounts  *inmem.AccountRepository
	limiter   *limiter
	now       time.Time

	following     *inmem.FollowingRepository
	subscriptions *inmem.SubscriptionRepository
	news          *inmem.RemotePostRepository
	finder        finder
	media         *media
	frames        *frames
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	user := uuid.New()
	f := &fixture{
		site:   site,
		user:   user,
		people: people{user: {ID: user, Nickname: "vexner", Name: "Иван Петров", Status: "hi"}},
		posts:  posts{},
		resolver: &resolver{
			docs: map[fedi.URL]fedi.RemoteDoc{remoteActor: {
				ID:          remoteActor.String(),
				Inbox:       remoteActor.String() + "/inbox",
				SharedInbox: "https://m.example/inbox",
				KeyID:       remoteActor.String() + "#main-key",
				PublicKey:   "anya-key",
			}},
			gone: map[fedi.URL]bool{},
		},
		sender:    &sender{},
		followers: inmem.NewFollowerRepository(),
		accounts:  inmem.NewAccountRepository(),
		limiter:   &limiter{left: 1000},
		now:       time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC),

		following:     inmem.NewFollowingRepository(),
		subscriptions: inmem.NewSubscriptionRepository(),
		finder:        finder{"anya@m.example": remoteActor},
		media:         &media{},
		frames:        &frames{frames: map[fedi.URL][]byte{}},
	}
	f.news = inmem.NewRemotePostRepository(f.following, f.subscriptions)

	f.svc = service.New(service.Deps{
		Site:       site,
		Accounts:   f.accounts,
		Followers:  f.followers,
		Remotes:    inmem.NewRemoteRepository(),
		People:     f.people,
		Posts:      f.posts,
		Resolver:   f.resolver,
		Signatures: signatures{},
		Sender:     f.sender,
		Keys:       keys{},
		Limiter:    f.limiter,
		Tx:         inmem.TxManager{},
		Now:        func() time.Time { return f.now },

		Following:     f.following,
		Subscriptions: f.subscriptions,
		News:          f.news,
		Finder:        f.finder,
		Media:         f.media,
		Covers:        inmem.NewCoverRepository(f.news),
		Frames:        f.frames,
		Languages:     []string{"ru", "en"},
		Retention:     90 * 24 * time.Hour,
	})

	return f
}

func (f *fixture) open(t *testing.T) {
	t.Helper()

	if err := f.svc.Open(context.Background(), f.user); err != nil {
		t.Fatalf("Open: %v", err)
	}
}

// follow makes anya of m.example follow the page, signed with key.
func (f *fixture) follow(key string) error {
	a, err := fedi.NewIncoming(fedi.TypeFollow, "https://m.example/f/1", remoteActor.String(), f.site.Actor(f.user).String(), nil)
	if err != nil {
		return err
	}

	return f.svc.Receive(context.Background(), signedBy(key), a, f.user)
}

func signedBy(key string) service.SignedRequest {
	return service.SignedRequest{Headers: map[string]string{"signature": remoteActor.String() + "#main-key " + key}}
}

func TestOpenAndClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	st, err := f.svc.Status(ctx, f.user)
	if err != nil || st.Open || st.Domain != "freenet.example" {
		t.Fatalf("Status of a page never opened = %+v, %v", st, err)
	}

	f.open(t)
	f.open(t) // twice is fine

	if err = f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	if st, _ = f.svc.Status(ctx, f.user); !st.Open || st.Followers != 1 {
		t.Fatalf("Status of an open page = %+v", st)
	}

	f.sender.sent = nil

	if err = f.svc.Close(ctx, f.user); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(f.sender.sent) != 1 || f.sender.sent[0].activity.Type != fedi.TypeDelete || f.sender.sent[0].activity.Person != f.site.Actor(f.user) {
		t.Fatalf("closing sent %+v, want a Delete of the actor", f.sender.sent)
	}

	if st, _ = f.svc.Status(ctx, f.user); st.Open || st.Followers != 0 {
		t.Fatalf("Status of a closed page = %+v", st)
	}

	if _, err = f.svc.Actor(ctx, f.user); !errors.Is(err, fedi.ErrNotFederated) {
		t.Fatalf("Actor of a closed page = %v", err)
	}

	a, err := f.accounts.Get(ctx, f.user)
	if err != nil || a.Keys().Private != "our-private" {
		t.Fatalf("a closed page lost its keys: %v", err)
	}
}

func TestOpenHiddenPage(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.people[f.user] = service.Person{ID: f.user, Nickname: "vexner", Hidden: true}

	if err := f.svc.Open(context.Background(), f.user); !errors.Is(err, fedi.ErrPageHidden) {
		t.Fatalf("Open of a hidden page = %v", err)
	}
}

func TestActorAndFinger(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.svc.Finger(ctx, "acct:vexner@freenet.example"); !errors.Is(err, fedi.ErrNotFederated) {
		t.Fatalf("Finger of a page not open = %v", err)
	}

	f.open(t)

	p, err := f.svc.Finger(ctx, "acct:vexner@freenet.example")
	if err != nil {
		t.Fatalf("Finger: %v", err)
	}

	if p.ID != f.site.Actor(f.user) || p.Nickname != "vexner" || p.PublicKey != "our-public" || p.Summary != "<p>hi</p>" || p.Avatar != "" {
		t.Fatalf("Person = %+v", p)
	}

	for _, resource := range []string{"acct:vexner@other.example", "acct:nobody@freenet.example"} {
		if _, err = f.svc.Finger(ctx, resource); err == nil {
			t.Errorf("Finger(%q) found someone", resource)
		}
	}

	// A page closed to guests later is hidden from the Fediverse too.
	f.people[f.user] = service.Person{ID: f.user, Nickname: "vexner", Hidden: true}

	if _, err = f.svc.Actor(ctx, f.user); !errors.Is(err, fedi.ErrNotFederated) {
		t.Fatalf("Actor of a hidden page = %v", err)
	}
}

func TestNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	own := fedi.Post{ID: uuid.New(), AuthorID: f.user, WallID: f.user, Text: "x"}
	repost := fedi.Post{ID: uuid.New(), AuthorID: f.user, WallID: f.user, Repost: true}
	f.posts[own.ID], f.posts[repost.ID] = own, repost

	if _, err := f.svc.Note(ctx, own.ID); !errors.Is(err, fedi.ErrNotFederated) {
		t.Fatalf("Note of a page not open = %v", err)
	}

	f.open(t)

	n, err := f.svc.Note(ctx, own.ID)
	if err != nil || n.ID != f.site.Note(own.ID) {
		t.Fatalf("Note = %+v, %v", n, err)
	}

	if _, err = f.svc.Note(ctx, repost.ID); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Note of a repost = %v", err)
	}

	if _, err = f.svc.Note(ctx, uuid.New()); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Note of no post = %v", err)
	}
}

func TestFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	if err := f.follow("anya-key"); !errors.Is(err, fedi.ErrNotFederated) {
		t.Fatalf("following a page not open = %v", err)
	}

	f.open(t)

	if err := f.follow("forged-key"); !errors.Is(err, fedi.ErrBadSignature) {
		t.Fatalf("a forged follow = %v", err)
	}

	if n, _ := f.followers.Count(ctx, f.user); n != 0 {
		t.Fatal("a forged follow was taken")
	}

	if err := f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %+v, want one Accept", f.sender.sent)
	}

	accept := f.sender.sent[0]
	if accept.activity.Type != fedi.TypeAccept || accept.inbox != remoteActor+"/inbox" || accept.signerID != f.user ||
		accept.activity.Follow.ID != "https://m.example/f/1" {
		t.Fatalf("Accept = %+v", accept)
	}

	inboxes, _ := f.followers.Inboxes(ctx, f.user)
	if len(inboxes) != 1 || inboxes[0] != "https://m.example/inbox" {
		t.Fatalf("Inboxes = %v, want the shared inbox", inboxes)
	}
}

// A server that changed the key of its actor is read again once.
func TestFollowWithChangedKey(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.open(t)

	if err := f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	doc := f.resolver.docs[remoteActor]
	doc.PublicKey = "new-key"
	f.resolver.docs[remoteActor] = doc
	reads := f.resolver.reads

	if err := f.follow("new-key"); err != nil {
		t.Fatalf("follow with the new key: %v", err)
	}

	if f.resolver.reads != reads+1 {
		t.Fatalf("the actor was read %d times, want once", f.resolver.reads-reads)
	}
}

func TestUnfollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	f.open(t)

	if err := f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	inner, err := fedi.NewIncoming(fedi.TypeFollow, "https://m.example/f/1", remoteActor.String(), f.site.Actor(f.user).String(), nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	undo, err := fedi.NewIncoming(fedi.TypeUndo, "https://m.example/u/1", remoteActor.String(), "https://m.example/f/1", &inner)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	if err = f.svc.Receive(ctx, signedBy("forged"), undo, uuid.Nil); !errors.Is(err, fedi.ErrBadSignature) {
		t.Fatalf("a forged Undo = %v", err)
	}

	if err = f.svc.Receive(ctx, signedBy("anya-key"), undo, uuid.Nil); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if n, _ := f.followers.Count(ctx, f.user); n != 0 {
		t.Fatalf("followers after Undo = %d", n)
	}
}

func TestDeletedActor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	f.open(t)

	if err := f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	f.resolver.gone[remoteActor] = true
	f.now = f.now.Add(48 * time.Hour) // the document read before is stale

	del, err := fedi.NewIncoming(fedi.TypeDelete, "https://m.example/d/1", remoteActor.String(), remoteActor.String(), nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	if err = f.svc.Receive(ctx, signedBy("whatever"), del, uuid.Nil); err != nil {
		t.Fatalf("Delete of a gone actor: %v", err)
	}

	if n, _ := f.followers.Count(ctx, f.user); n != 0 {
		t.Fatalf("followers after the actor is gone = %d", n)
	}
}

func TestReceiveOther(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	like, err := fedi.NewIncoming("Like", "https://m.example/l/1", remoteActor.String(), "https://freenet.example/ap/posts/1", nil)
	if err != nil {
		t.Fatalf("NewIncoming: %v", err)
	}

	if err = f.svc.Receive(context.Background(), service.SignedRequest{}, like, uuid.Nil); err != nil {
		t.Fatalf("a Like = %v, want taken and dropped", err)
	}

	if f.resolver.reads != 0 {
		t.Fatal("an activity dropped cost a request to its server")
	}

	f.limiter.left = 0

	if err = f.svc.Receive(context.Background(), service.SignedRequest{}, like, uuid.Nil); !errors.Is(err, fedi.ErrTooMany) {
		t.Fatalf("an activity over the limit = %v", err)
	}
}

func TestPostChanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	post := fedi.Post{ID: uuid.New(), AuthorID: f.user, WallID: f.user, Text: "hello"}
	f.posts[post.ID] = post

	published := service.PostEvent{Change: service.PostPublished, PostID: post.ID, AuthorID: f.user}

	if err := f.svc.PostChanged(ctx, published); err != nil || len(f.sender.sent) != 0 {
		t.Fatalf("a post of a page not open: %v, sent %d", err, len(f.sender.sent))
	}

	f.open(t)

	if err := f.svc.PostChanged(ctx, published); err != nil || len(f.sender.sent) != 0 {
		t.Fatalf("a post of a page nobody follows: %v, sent %d", err, len(f.sender.sent))
	}

	if err := f.follow("anya-key"); err != nil {
		t.Fatalf("follow: %v", err)
	}
	f.sender.sent = nil

	tests := []struct {
		name  string
		event service.PostEvent
		post  fedi.Post
		want  string
	}{
		{"published", published, post, fedi.TypeCreate},
		{"edited", service.PostEvent{Change: service.PostEdited, PostID: post.ID, AuthorID: f.user}, post, fedi.TypeUpdate},
		{"deleted", service.PostEvent{Change: service.PostDeleted, PostID: post.ID, AuthorID: f.user}, post, fedi.TypeDelete},
		{"archived", published, fedi.Post{ID: post.ID, AuthorID: f.user, WallID: f.user, Archived: true}, ""},
		{"gone", service.PostEvent{Change: service.PostPublished, PostID: uuid.New(), AuthorID: f.user}, post, ""},
	}

	for _, tt := range tests {
		f.sender.sent = nil
		f.posts[post.ID] = tt.post

		if err := f.svc.PostChanged(ctx, tt.event); err != nil {
			t.Fatalf("%s: PostChanged: %v", tt.name, err)
		}

		if tt.want == "" {
			if len(f.sender.sent) != 0 {
				t.Errorf("%s: sent %+v, want nothing", tt.name, f.sender.sent)
			}
			continue
		}

		if len(f.sender.sent) != 1 || f.sender.sent[0].activity.Type != tt.want || f.sender.sent[0].inbox != "https://m.example/inbox" {
			t.Errorf("%s: sent %+v, want one %s to the shared inbox", tt.name, f.sender.sent, tt.want)
		}
	}
}
