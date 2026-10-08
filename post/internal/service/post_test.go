package service_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// fakeNotifier records what the services tell the notification service,
// and fails with err when set.
type fakeNotifier struct {
	mu        sync.Mutex
	notified  []service.Event
	retracted []service.Event
	forgotten []uuid.UUID
	// forgottenComments are the comments whose notifications went.
	forgottenComments []uuid.UUID
	outsideTx         int
	err               error
}

func (f *fakeNotifier) record(ctx context.Context, add func()) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}
	if f.err != nil {
		return f.err
	}

	add()

	return nil
}

func (f *fakeNotifier) Notify(ctx context.Context, e service.Event) error {
	return f.record(ctx, func() { f.notified = append(f.notified, e) })
}

func (f *fakeNotifier) Retract(ctx context.Context, e service.Event) error {
	return f.record(ctx, func() { f.retracted = append(f.retracted, e) })
}

func (f *fakeNotifier) ForgetPost(ctx context.Context, postID uuid.UUID) error {
	return f.record(ctx, func() { f.forgotten = append(f.forgotten, postID) })
}

func (f *fakeNotifier) ForgetComment(ctx context.Context, commentID uuid.UUID) error {
	return f.record(ctx, func() { f.forgottenComments = append(f.forgottenComments, commentID) })
}

// photoCall is what PhotoKeeper was told about whose photos.
type photoCall struct {
	owner  uuid.UUID
	photos []uuid.UUID
}

// fakePhotos records the photos published and deleted, and in how many
// calls it was not in a transaction.
type fakePhotos struct {
	mu        sync.Mutex
	published []photoCall
	deleted   []photoCall
	outsideTx int
	err       error
}

func (f *fakePhotos) record(ctx context.Context, to *[]photoCall, c photoCall) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}
	if f.err != nil {
		return f.err
	}

	*to = append(*to, c)

	return nil
}

func (f *fakePhotos) Publish(ctx context.Context, owner uuid.UUID, photos []uuid.UUID) error {
	return f.record(ctx, &f.published, photoCall{owner, photos})
}

func (f *fakePhotos) Delete(ctx context.Context, owner uuid.UUID, photos []uuid.UUID) error {
	return f.record(ctx, &f.deleted, photoCall{owner, photos})
}

// fakeModerators makes users admins of community walls, keyed by wall and
// user; the empty one moderates nothing, as for the walls of users.
type fakeModerators map[[2]uuid.UUID]bool

func (f fakeModerators) CanModerate(_ context.Context, wallID, userID uuid.UUID) (bool, error) {
	return f[[2]uuid.UUID{wallID, userID}], nil
}

type inTx struct{}

// fakeTx marks the context of the transaction, so that the notifier can
// tell it was called inside one.
type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, inTx{}, true))
}

func newPostService() *service.PostService {
	store := inmem.NewStore()
	return service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{}})
}

func write(t *testing.T, svc *service.PostService, author uuid.UUID, texts ...string) []*post.Post {
	t.Helper()

	posts := make([]*post.Post, 0, len(texts))
	for _, text := range texts {
		content, err := post.NewContent(post.Text(text), nil)
		if err != nil {
			t.Fatal(err)
		}

		p, err := svc.Create(context.Background(), service.CreateInput{WallID: author, AuthorID: author, Content: content})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		posts = append(posts, p)
	}

	return posts
}

// textContent is text-only content.
func textContent(t *testing.T, text string) post.Content {
	t.Helper()

	content, err := post.NewContent(post.Text(text), nil)
	if err != nil {
		t.Fatal(err)
	}

	return content
}

func texts(posts []*post.Post) []string {
	out := make([]string, len(posts))
	for i, p := range posts {
		out[i] = p.Content().Text().String()
	}
	return out
}

func TestPostService_Delete(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	author := uuid.New()
	p := write(t, svc, author, "привет")[0]

	if err := svc.Delete(ctx, uuid.New(), p.ID()); !errors.Is(err, post.ErrCannotDelete) {
		t.Fatalf("stranger: got %v, want ErrCannotDelete", err)
	}

	if err := svc.Delete(ctx, author, p.ID()); err != nil {
		t.Fatalf("author: %v", err)
	}

	if err := svc.Delete(ctx, author, p.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("twice: got %v, want ErrNotFound", err)
	}
}

func TestPostService_ListWall(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	write(t, svc, ann, "1", "2", "3", "4", "5")
	write(t, svc, bob, "чужое")

	var (
		got    [][]string
		cursor uuid.UUID
	)

	for {
		page, err := svc.ListWall(ctx, uuid.Nil, ann, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Fatalf("total = %d, want 5", page.Total)
		}

		got = append(got, texts(page.Posts))

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	want := [][]string{{"5", "4"}, {"3", "2"}, {"1"}}
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || got[i][0] != want[i][0] || got[i][len(got[i])-1] != want[i][len(want[i])-1] {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}

	t.Run("exact multiple of the page size ends without a cursor", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, ann, uuid.Nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 || page.NextCursor != uuid.Nil {
			t.Fatalf("posts = %d, next = %s", len(page.Posts), page.NextCursor)
		}
	})

	t.Run("bad size falls back to the default", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, ann, uuid.Nil, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 {
			t.Fatalf("posts = %d", len(page.Posts))
		}
	})

	t.Run("empty wall", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, uuid.New(), uuid.Nil, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 0 || page.Total != 0 || page.NextCursor != uuid.Nil {
			t.Fatalf("page = %+v", page)
		}
	})
}

func TestPostService_Feed(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	write(t, svc, ann, "ann 1")
	write(t, svc, bob, "bob 1")
	write(t, svc, cat, "cat: not followed")
	write(t, svc, ann, "ann 2")

	first, err := svc.Feed(ctx, uuid.Nil, service.FeedSources{Authors: []uuid.UUID{ann, bob}}, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(first.Posts); len(got) != 2 || got[0] != "ann 2" || got[1] != "bob 1" || first.NextCursor == uuid.Nil {
		t.Fatalf("first page: %v", got)
	}

	rest, err := svc.Feed(ctx, uuid.Nil, service.FeedSources{Authors: []uuid.UUID{ann, bob}}, first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(rest.Posts); len(got) != 1 || got[0] != "ann 1" || rest.NextCursor != uuid.Nil {
		t.Fatalf("last page: %v", got)
	}

	empty, err := svc.Feed(ctx, uuid.Nil, service.FeedSources{}, uuid.Nil, 20)
	if err != nil || len(empty.Posts) != 0 {
		t.Fatalf("no authors: %+v, %v", empty, err)
	}

	if _, err = svc.Feed(ctx, uuid.Nil, service.FeedSources{Authors: make([]uuid.UUID, service.MaxFeedAuthors+1)}, uuid.Nil, 20); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many authors: %v", err)
	}
}

func TestPostService_Likes(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	posts := write(t, svc, ann, "первая", "вторая")
	first, second := posts[0], posts[1]

	steps := []struct {
		name string
		do   func(ctx context.Context, userID, postID uuid.UUID) (int, error)
		user uuid.UUID
		want int
	}{
		{"ann likes her post", svc.Like, ann, 1},
		{"bob likes it too", svc.Like, bob, 2},
		{"a second like counts once", svc.Like, bob, 2},
		{"bob takes his back", svc.Unlike, bob, 1},
		{"unliking twice changes nothing", svc.Unlike, bob, 1},
	}

	for _, step := range steps {
		likes, err := step.do(ctx, step.user, first.ID())
		if err != nil || likes != step.want {
			t.Fatalf("%s: likes = %d, %v; want %d", step.name, likes, err, step.want)
		}
	}

	p, liked, err := svc.Get(ctx, ann, first.ID())
	if err != nil || !liked || p.Counters().Likes != 1 {
		t.Fatalf("Get by ann = %v, %v, %v", p.Counters(), liked, err)
	}
	if _, liked, _ = svc.Get(ctx, bob, first.ID()); liked {
		t.Fatal("bob took his like back")
	}
	if _, liked, _ = svc.Get(ctx, uuid.Nil, first.ID()); liked {
		t.Fatal("an anonymous viewer likes nothing")
	}

	wall, err := svc.ListWall(ctx, ann, ann, uuid.Nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !wall.Liked[first.ID()] || wall.Liked[second.ID()] {
		t.Fatalf("wall liked = %v", wall.Liked)
	}

	feed, err := svc.Feed(ctx, ann, service.FeedSources{Authors: []uuid.UUID{ann}}, uuid.Nil, 20)
	if err != nil || !feed.Liked[first.ID()] || len(feed.Liked) != 1 {
		t.Fatalf("feed liked = %v, %v", feed.Liked, err)
	}

	anonymous, err := svc.ListWall(ctx, uuid.Nil, ann, uuid.Nil, 20)
	if err != nil || len(anonymous.Liked) != 0 {
		t.Fatalf("anonymous liked = %v, %v", anonymous.Liked, err)
	}

	if _, err = svc.Like(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("like a missing post: %v", err)
	}
	if _, err = svc.Unlike(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("unlike a missing post: %v", err)
	}
	if _, _, err = svc.Get(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("get a missing post: %v", err)
	}
}

func TestPostService_NotifiesAboutLikes(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, svc, ann, "привет")[0]

	for _, step := range []func(context.Context, uuid.UUID, uuid.UUID) (int, error){svc.Like, svc.Unlike} {
		if _, err := step(ctx, bob, p.ID()); err != nil {
			t.Fatal(err)
		}
	}

	want := service.Event{Kind: service.EventLike, RecipientID: ann, ActorID: bob, PostID: p.ID()}
	if len(notifier.notified) != 1 || notifier.notified[0] != want {
		t.Fatalf("notified = %+v, want %+v", notifier.notified, want)
	}
	if len(notifier.retracted) != 1 || notifier.retracted[0] != want {
		t.Fatalf("retracted = %+v, want %+v", notifier.retracted, want)
	}

	// Nobody is notified about liking their own post.
	if _, err := svc.Like(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.notified) != 1 {
		t.Fatalf("own like notified: %+v", notifier.notified)
	}

	if err := svc.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.forgotten) != 1 || notifier.forgotten[0] != p.ID() {
		t.Fatalf("forgotten = %v", notifier.forgotten)
	}

	// A failed like notifies nobody.
	if _, err := svc.Like(ctx, bob, p.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("like a deleted post: %v", err)
	}
	if len(notifier.notified) != 1 {
		t.Fatalf("notified about a failed like: %+v", notifier.notified)
	}

	if notifier.outsideTx != 0 {
		t.Errorf("%d notifications recorded outside the transaction of their change", notifier.outsideTx)
	}
}

// A notification that cannot be recorded fails its change: the two are
// saved together or not at all.
func TestPostService_NotifierFailureFailsTheChange(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, svc, ann, "привет")[0]

	notifier.err = errors.New("outbox is down")

	if _, err := svc.Like(ctx, bob, p.ID()); !errors.Is(err, notifier.err) {
		t.Errorf("Like = %v, want the notifier's error", err)
	}
	content, err := post.NewContent("на стене", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: content}); !errors.Is(err, notifier.err) {
		t.Errorf("Create on another's wall = %v, want the notifier's error", err)
	}
	if err := svc.Delete(ctx, ann, p.ID()); !errors.Is(err, notifier.err) {
		t.Errorf("Delete = %v, want the notifier's error", err)
	}
}

func TestPostService_GetMany(t *testing.T) {
	t.Parallel()

	svc := newPostService()
	ctx := context.Background()
	posts := write(t, svc, uuid.New(), "раз", "два")

	got, err := svc.GetMany(ctx, []uuid.UUID{posts[0].ID(), uuid.New(), posts[1].ID()})
	if err != nil || len(got) != 2 {
		t.Fatalf("GetMany = %v, %v; missing posts are left out", got, err)
	}

	if got, err = svc.GetMany(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("no ids: %v, %v", got, err)
	}

	if _, err = svc.GetMany(ctx, make([]uuid.UUID, service.MaxPageSize+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many ids: %v", err)
	}
}

func TestPostService_SomeoneElsesWall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}})

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	write(t, svc, ann, "своё")

	content, err := post.NewContent("привет, Аня", nil)
	if err != nil {
		t.Fatal(err)
	}

	p, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if p.WallID() != ann || p.AuthorID() != bob {
		t.Fatalf("wall = %v, author = %v", p.WallID(), p.AuthorID())
	}

	want := service.Event{Kind: service.EventWallPost, RecipientID: ann, ActorID: bob, PostID: p.ID()}
	if len(notifier.notified) != 1 || notifier.notified[0] != want {
		t.Fatalf("notified = %+v, want only %+v (own posts notify nobody)", notifier.notified, want)
	}

	wall, err := svc.ListWall(ctx, uuid.Nil, ann, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(wall.Posts); wall.Total != 2 || len(got) != 2 || got[0] != "привет, Аня" {
		t.Fatalf("ann's wall = %v (total %d), want bob's post on it", got, wall.Total)
	}

	bobs, err := svc.ListWall(ctx, uuid.Nil, bob, uuid.Nil, 10)
	if err != nil || bobs.Total != 0 {
		t.Fatalf("bob's wall = %+v, %v; want empty", bobs, err)
	}

	feed, err := svc.Feed(ctx, uuid.Nil, service.FeedSources{Authors: []uuid.UUID{bob}}, uuid.Nil, 10)
	if err != nil || len(feed.Posts) != 1 || feed.Posts[0].ID() != p.ID() {
		t.Fatalf("bob's posts in a feed = %+v, %v", feed.Posts, err)
	}

	if err = svc.Delete(ctx, cat, p.ID()); !errors.Is(err, post.ErrCannotDelete) {
		t.Fatalf("stranger: got %v, want ErrCannotDelete", err)
	}
	if err = svc.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatalf("the owner of the wall: %v", err)
	}
	if len(notifier.forgotten) != 1 || notifier.forgotten[0] != p.ID() {
		t.Fatalf("forgotten = %v", notifier.forgotten)
	}
}

func TestPostService_Repost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}})

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	original := write(t, svc, ann, "оригинал")[0]

	shared, err := svc.Repost(ctx, bob, original.ID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if shared.WallID() != bob || shared.AuthorID() != bob || shared.Content().RepostOf() != original.ID() {
		t.Fatalf("bob's repost: wall %v, author %v, shares %v", shared.WallID(), shared.AuthorID(), shared.Content().RepostOf())
	}

	want := service.Event{Kind: service.EventRepost, RecipientID: ann, ActorID: bob, PostID: shared.ID()}
	if len(notifier.notified) != 1 || notifier.notified[0] != want {
		t.Fatalf("notified = %+v, want %+v", notifier.notified, want)
	}

	// Cat shares bob's repost without a word: the original is shared.
	passed, err := svc.Repost(ctx, cat, shared.ID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if passed.Content().RepostOf() != original.ID() {
		t.Fatalf("a repost of an empty repost shares %v, want the original", passed.Content().RepostOf())
	}

	// A repost with a word of its own is shared itself.
	said, err := svc.Repost(ctx, cat, original.ID(), "согласна")
	if err != nil {
		t.Fatal(err)
	}
	ownWord, err := svc.Repost(ctx, bob, said.ID(), "")
	if err != nil || ownWord.Content().RepostOf() != said.ID() {
		t.Fatalf("sharing a repost with a word: %v, %v", ownWord, err)
	}

	got, _, err := svc.Get(ctx, uuid.Nil, original.ID())
	if err != nil || got.Counters().Reposts != 3 {
		t.Fatalf("original reposts = %+v, %v; want 3", got.Counters(), err)
	}

	if _, err = svc.Repost(ctx, ann, original.ID(), "моё"); err != nil {
		t.Fatal(err)
	}
	if n := len(notifier.notified); n != 4 {
		t.Fatalf("notifications = %d, want 4 (own reposts notify nobody)", n)
	}

	if err = svc.Delete(ctx, bob, shared.ID()); err != nil {
		t.Fatal(err)
	}
	if got, _, _ = svc.Get(ctx, uuid.Nil, original.ID()); got.Counters().Reposts != 3 {
		t.Fatalf("after deleting a repost: %+v", got.Counters())
	}
	if f := notifier.forgotten; len(f) != 1 || f[0] != shared.ID() {
		t.Fatalf("forgotten = %v, want the repost (its notification goes)", f)
	}

	if _, err = svc.Repost(ctx, bob, uuid.New(), ""); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("sharing nothing: %v, want ErrNotFound", err)
	}
}

func TestPostService_Photos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: photos, Moderators: fakeModerators{}})
	owner, author := uuid.New(), uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	withPhotos, err := post.NewContent("на стене", ids)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(ctx, service.CreateInput{WallID: owner, AuthorID: author, Content: withPhotos})
	if err != nil {
		t.Fatal(err)
	}
	write(t, svc, author, "без фото")

	if len(photos.published) != 1 || photos.published[0].owner != author || !slices.Equal(photos.published[0].photos, ids) {
		t.Fatalf("published = %+v, want the author's two photos once", photos.published)
	}

	// The owner of the wall deletes it: the photos are the author's still.
	if err = svc.Delete(ctx, owner, p.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != author || !slices.Equal(photos.deleted[0].photos, ids) {
		t.Fatalf("deleted = %+v, want the author's two photos", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d photo calls outside the transaction", photos.outsideTx)
	}

	// Photos that cannot be recorded fail the post, which is not saved.
	photos.err = errors.New("outbox is down")
	if _, err = svc.Create(ctx, service.CreateInput{WallID: author, AuthorID: author, Content: withPhotos}); !errors.Is(err, photos.err) {
		t.Fatalf("Create with a broken outbox: %v", err)
	}
}

func TestPostService_Edit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: photos, Moderators: fakeModerators{}})
	author, stranger := uuid.New(), uuid.New()
	kept, dropped, added := uuid.New(), uuid.New(), uuid.New()

	content, err := post.NewContent("было", post.Photos{kept, dropped})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(ctx, service.CreateInput{WallID: author, AuthorID: author, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Like(ctx, author, p.ID()); err != nil {
		t.Fatal(err)
	}
	photos.published = nil

	if _, _, err = svc.Edit(ctx, stranger, p.ID(), "чужое", nil); !errors.Is(err, post.ErrCannotEdit) {
		t.Fatalf("a stranger edits: %v", err)
	}

	edited, liked, err := svc.Edit(ctx, author, p.ID(), "стало", post.Photos{added, kept})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if !liked || edited.EditedAt().IsZero() || edited.Content().Text() != "стало" {
		t.Fatalf("edited = %+v, liked %v", edited.Content(), liked)
	}

	got, _, err := svc.Get(ctx, uuid.Nil, p.ID())
	if err != nil || got.Content().Text() != "стало" || !slices.Equal(got.Content().Photos(), post.Photos{added, kept}) || got.Counters().Likes != 1 {
		t.Fatalf("stored post = %+v, %+v, %v", got.Content(), got.Counters(), err)
	}

	// Media keeps the new photo and deletes the one taken away, in the
	// transaction of the edit.
	if len(photos.published) != 1 || !slices.Equal(photos.published[0].photos, []uuid.UUID{added}) {
		t.Errorf("published = %+v, want the added photo", photos.published)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != author || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{dropped}) {
		t.Errorf("deleted = %+v, want the dropped photo", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Errorf("%d photo calls outside the transaction", photos.outsideTx)
	}

	if _, _, err = svc.Edit(ctx, author, uuid.New(), "нет", nil); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("a missing post: %v", err)
	}
}

func TestPostService_Pin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newPostService()
	owner, guest := uuid.New(), uuid.New()
	posts := write(t, svc, owner, "первая", "вторая", "третья", "четвёртая")
	first, third := posts[0], posts[2]

	if err := svc.Pin(ctx, guest, first.ID()); !errors.Is(err, post.ErrCannotPin) {
		t.Fatalf("a guest pins: %v", err)
	}
	if err := svc.Pin(ctx, owner, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("pin a missing post: %v", err)
	}

	for range 2 { // idempotent
		if err := svc.Pin(ctx, owner, first.ID()); err != nil {
			t.Fatalf("Pin: %v", err)
		}
	}

	page, err := svc.ListWall(ctx, uuid.Nil, owner, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(page.Posts); !slices.Equal(got, []string{"первая", "четвёртая", "третья"}) || !page.Posts[0].Pinned() {
		t.Fatalf("first page = %v; want the pinned post on top of two others", got)
	}
	if page.Total != 4 {
		t.Fatalf("total = %d, the pinned post counts", page.Total)
	}

	next, err := svc.ListWall(ctx, uuid.Nil, owner, page.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(next.Posts); !slices.Equal(got, []string{"вторая"}) {
		t.Fatalf("next page = %v; the pinned post is not shown twice", got)
	}

	// Pinning another replaces it; unpinning returns it to its place.
	if err = svc.Pin(ctx, owner, third.ID()); err != nil {
		t.Fatal(err)
	}
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, uuid.Nil, 10); err != nil {
		t.Fatal(err)
	}
	if got := texts(page.Posts); !slices.Equal(got, []string{"третья", "четвёртая", "вторая", "первая"}) {
		t.Fatalf("after pinning the third = %v", got)
	}

	if err = svc.Unpin(ctx, guest, third.ID()); !errors.Is(err, post.ErrCannotPin) {
		t.Fatalf("a guest unpins: %v", err)
	}
	for range 2 {
		if err = svc.Unpin(ctx, owner, third.ID()); err != nil {
			t.Fatalf("Unpin: %v", err)
		}
	}
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, uuid.Nil, 10); err != nil {
		t.Fatal(err)
	}
	if got := texts(page.Posts); !slices.Equal(got, []string{"четвёртая", "третья", "вторая", "первая"}) || page.Posts[1].Pinned() {
		t.Fatalf("after unpinning = %v", got)
	}

	// A deleted pinned post takes its pin with it.
	if err = svc.Pin(ctx, owner, first.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(ctx, owner, first.ID()); err != nil {
		t.Fatal(err)
	}
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, uuid.Nil, 10); err != nil || len(page.Posts) != 3 {
		t.Fatalf("after deleting the pinned post: %v, %v", texts(page.Posts), err)
	}
}

func TestPostService_ClubWalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	club, admin, member, stranger := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	moderators := fakeModerators{{club, admin}: true}
	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{},
		Moderators: moderators,
	})
	comments := service.NewCommentService(service.CommentDeps{
		Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{},
		Moderators: moderators,
	})
	content := textContent(t, "новости")

	news, err := posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: admin, Content: content, ClubWall: true, FromWall: true})
	if err != nil || !news.FromWall() {
		t.Fatalf("Create from the wall = %+v, %v", news, err)
	}
	byMember, err := posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: member, Content: content, ClubWall: true})
	if err != nil || byMember.FromWall() {
		t.Fatalf("Create by a member = %+v, %v", byMember, err)
	}
	if _, err = posts.Create(ctx, service.CreateInput{WallID: admin, AuthorID: admin, Content: content, FromWall: true}); !errors.Is(err, post.ErrFromOwnWall) {
		t.Fatalf("from one's own wall: %v", err)
	}

	// Nobody owns a community's wall to hear of posts on it, and a post
	// that speaks for the community tells its author nothing.
	if _, err = posts.Like(ctx, member, news.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = comments.Create(ctx, member, news.ID(), post.RestoreCommentContent("ура", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err = posts.Repost(ctx, member, news.ID(), ""); err != nil {
		t.Fatal(err)
	}
	if len(notifier.notified) != 0 {
		t.Fatalf("notified = %+v, want nobody", notifier.notified)
	}

	// The member's own post on the club's wall notifies the member.
	if _, err = posts.Like(ctx, stranger, byMember.ID()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.notified) != 1 || notifier.notified[0].RecipientID != member {
		t.Fatalf("notified = %+v, want the member", notifier.notified)
	}

	// The admins keep the wall clean; strangers do not.
	strangers, err := comments.Create(ctx, stranger, byMember.ID(), post.RestoreCommentContent("спам", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Delete(ctx, member, strangers.ID()); err != nil {
		t.Fatalf("the author of the post deletes a comment: %v", err)
	}
	if strangers, err = comments.Create(ctx, stranger, news.ID(), post.RestoreCommentContent("спам", nil)); err != nil {
		t.Fatal(err)
	}
	if err = comments.Delete(ctx, member, strangers.ID()); !errors.Is(err, post.ErrCannotDeleteComment) {
		t.Fatalf("a member deletes a comment: %v", err)
	}
	if err = comments.Delete(ctx, admin, strangers.ID()); err != nil {
		t.Fatalf("the admin deletes a comment: %v", err)
	}

	if err = posts.Pin(ctx, member, byMember.ID()); !errors.Is(err, post.ErrCannotPin) {
		t.Fatalf("a member pins: %v", err)
	}
	if err = posts.Pin(ctx, admin, byMember.ID()); err != nil {
		t.Fatalf("the admin pins: %v", err)
	}
	if err = posts.Delete(ctx, stranger, byMember.ID()); !errors.Is(err, post.ErrCannotDelete) {
		t.Fatalf("a stranger deletes: %v", err)
	}
	if err = posts.Delete(ctx, admin, byMember.ID()); err != nil {
		t.Fatalf("the admin deletes: %v", err)
	}
}

func TestPostService_FeedOfClubs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newPostService()
	club, admin, member, ann := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	content := textContent(t, "x")

	mustCreate := func(in service.CreateInput) *post.Post {
		t.Helper()
		in.Content = content
		p, err := svc.Create(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	news := mustCreate(service.CreateInput{WallID: club, AuthorID: admin, ClubWall: true, FromWall: true})
	byMember := mustCreate(service.CreateInput{WallID: club, AuthorID: member, ClubWall: true})
	adminsOwn := mustCreate(service.CreateInput{WallID: admin, AuthorID: admin})
	mustCreate(service.CreateInput{WallID: ann, AuthorID: ann})

	// Following the admin shows their own posts, not those that speak for
	// the club: who wrote those is the club's secret.
	page, err := svc.Feed(ctx, ann, service.FeedSources{Authors: []uuid.UUID{admin}}, uuid.Nil, 20)
	if err != nil || len(page.Posts) != 1 || page.Posts[0].ID() != adminsOwn.ID() {
		t.Fatalf("feed of the admin = %v, %v", ids(page.Posts), err)
	}

	// Being in the club shows everything on its wall.
	page, err = svc.Feed(ctx, ann, service.FeedSources{Walls: []uuid.UUID{club}}, uuid.Nil, 20)
	if err != nil || len(page.Posts) != 2 || page.Posts[0].ID() != byMember.ID() || page.Posts[1].ID() != news.ID() {
		t.Fatalf("feed of the club = %v, %v", ids(page.Posts), err)
	}

	if _, err = svc.Feed(ctx, ann, service.FeedSources{Walls: make([]uuid.UUID, service.MaxFeedAuthors+1)}, uuid.Nil, 20); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many clubs: %v", err)
	}
}

func ids(posts []*post.Post) []uuid.UUID {
	out := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		out[i] = p.ID()
	}
	return out
}
