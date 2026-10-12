package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	copied    []service.PhotoCopy
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

func (f *fakePhotos) Copy(ctx context.Context, _ uuid.UUID, copies []service.PhotoCopy) error {
	var call []photoCall
	if err := f.record(ctx, &call, photoCall{}); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = append(f.copied, copies...)

	return nil
}

// fakeModerators makes users admins of community walls, keyed by wall and
// user; the empty one moderates nothing, as for the walls of users.
type fakeModerators map[[2]uuid.UUID]bool

func (f fakeModerators) CanModerate(_ context.Context, wallID, userID uuid.UUID) (bool, error) {
	return f[[2]uuid.UUID{wallID, userID}], nil
}

func (f fakeModerators) Moderators(_ context.Context, wallID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for key, ok := range f {
		if key[0] == wallID && ok {
			ids = append(ids, key[1])
		}
	}

	return ids, nil
}

type inTx struct{}

// fakeTx marks the context of the transaction, so that the notifier can
// tell it was called inside one.
type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, inTx{}, true))
}

func newPostService() *service.PostService {
	svc, _ := newFeedService()
	return svc
}

// follow makes a reader read people in the news and, after them, clubs.
type follow func(reader uuid.UUID, people []uuid.UUID, clubs ...uuid.UUID)

// newFeedService is newPostService with the way to say whom its readers
// read, as the user and club services tell post.
func newFeedService() (*service.PostService, follow) {
	store := inmem.NewStore()
	sources := service.NewSourceService(store.Sources())

	reads := func(reader uuid.UUID, people []uuid.UUID, clubs ...uuid.UUID) {
		for _, id := range people {
			if err := sources.Follow(context.Background(), reader, id, false); err != nil {
				panic(err)
			}
		}
		for _, id := range clubs {
			if err := sources.Follow(context.Background(), reader, id, true); err != nil {
				panic(err)
			}
		}
	}

	return service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{}, Switches: store.Posts(), Polls: store.Polls()}), reads
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
		page, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: post.WallAll}, cursor, 2)
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

	t.Run("the tabs of the wall", func(t *testing.T) {
		svc := newPostService()
		ann, bob := uuid.New(), uuid.New()
		posts := write(t, svc, ann, "1", "2")

		content, _ := post.NewContent("в гостях", nil)
		if _, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: content}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Pin(ctx, ann, posts[0].ID()); err != nil {
			t.Fatal(err)
		}

		for _, tt := range []struct {
			filter post.WallFilter
			want   []string
		}{
			{post.WallAll, []string{"1", "в гостях", "2"}}, // pinned first
			{post.WallOwner, []string{"2", "1"}},           // pinned in its place
			{post.WallOthers, []string{"в гостях"}},
		} {
			page, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: tt.filter}, uuid.Nil, 10)
			if err != nil || !slices.Equal(texts(page.Posts), tt.want) || page.Total != len(tt.want) {
				t.Errorf("%q: %v (total %d), %v", tt.filter, texts(page.Posts), page.Total, err)
			}
		}
	})

	t.Run("exact multiple of the page size ends without a cursor", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: post.WallAll}, uuid.Nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 || page.NextCursor != uuid.Nil {
			t.Fatalf("posts = %d, next = %s", len(page.Posts), page.NextCursor)
		}
	})

	t.Run("bad size falls back to the default", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: post.WallAll}, uuid.Nil, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Posts) != 5 {
			t.Fatalf("posts = %d", len(page.Posts))
		}
	})

	t.Run("empty wall", func(t *testing.T) {
		page, err := svc.ListWall(ctx, uuid.Nil, uuid.New(), service.WallPick{Filter: post.WallAll}, uuid.Nil, 20)
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

	svc, reads := newFeedService()
	ctx := context.Background()
	ann, bob, cat, reader := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	reads(reader, []uuid.UUID{ann, bob})

	write(t, svc, ann, "ann 1")
	write(t, svc, bob, "bob 1")
	write(t, svc, cat, "cat: not followed")
	write(t, svc, ann, "ann 2")

	first, err := svc.Feed(ctx, reader, service.FeedSources{}, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(first.Posts); len(got) != 2 || got[0] != "ann 2" || got[1] != "bob 1" || first.NextCursor == uuid.Nil {
		t.Fatalf("first page: %v", got)
	}

	rest, err := svc.Feed(ctx, reader, service.FeedSources{}, first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(rest.Posts); len(got) != 1 || got[0] != "ann 1" || rest.NextCursor != uuid.Nil {
		t.Fatalf("last page: %v", got)
	}

	// Nobody's news are empty; who follows nobody reads their own posts.
	empty, err := svc.Feed(ctx, uuid.Nil, service.FeedSources{}, uuid.Nil, 20)
	if err != nil || len(empty.Posts) != 0 {
		t.Fatalf("no reader: %+v, %v", empty, err)
	}
	own, err := svc.Feed(ctx, cat, service.FeedSources{}, uuid.Nil, 20)
	if got := texts(own.Posts); err != nil || len(got) != 1 || got[0] != "cat: not followed" {
		t.Fatalf("who follows nobody: %v, %v", got, err)
	}

	if _, err = svc.Feed(ctx, reader, service.FeedSources{Hidden: make([]uuid.UUID, service.MaxFeedHidden+1)}, uuid.Nil, 20); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many hidden authors: %v", err)
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

	wall, err := svc.ListWall(ctx, ann, ann, service.WallPick{Filter: post.WallAll}, uuid.Nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !wall.Liked[first.ID()] || wall.Liked[second.ID()] {
		t.Fatalf("wall liked = %v", wall.Liked)
	}

	feed, err := svc.Feed(ctx, ann, service.FeedSources{}, uuid.Nil, 20)
	if err != nil || !feed.Liked[first.ID()] || len(feed.Liked) != 1 {
		t.Fatalf("feed liked = %v, %v", feed.Liked, err)
	}

	anonymous, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: post.WallAll}, uuid.Nil, 20)
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

	// Bob names ann, cat and himself on ann's wall: ann hears of the
	// post, as its owner, and cat of her name.
	notifier.notified = nil
	named, err := svc.Create(ctx, service.CreateInput{
		WallID: ann, AuthorID: bob, Content: post.RestoreContent("@ann и @cat", nil, uuid.Nil), Mentions: post.Mentions{ann, cat, bob},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNamed := []service.Event{
		{Kind: service.EventWallPost, RecipientID: ann, ActorID: bob, PostID: named.ID()},
		{Kind: service.EventPostMention, RecipientID: cat, ActorID: bob, PostID: named.ID()},
	}
	if !slices.Equal(notifier.notified, wantNamed) {
		t.Fatalf("notified = %+v, want %+v", notifier.notified, wantNamed)
	}
	if err = svc.Delete(ctx, bob, named.ID()); err != nil {
		t.Fatal(err)
	}
	notifier.forgotten = nil

	wall, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(wall.Posts); wall.Total != 2 || len(got) != 2 || got[0] != "привет, Аня" {
		t.Fatalf("ann's wall = %v (total %d), want bob's post on it", got, wall.Total)
	}

	bobs, err := svc.ListWall(ctx, uuid.Nil, bob, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10)
	if err != nil || bobs.Total != 0 {
		t.Fatalf("bob's wall = %+v, %v; want empty", bobs, err)
	}

	// Whoever follows bob reads what he wrote on ann's wall.
	sources := service.NewSourceService(store.Sources())
	bobsReader, annsReader := uuid.New(), uuid.New()
	if err = sources.Follow(ctx, bobsReader, bob, false); err != nil {
		t.Fatal(err)
	}

	feed, err := svc.Feed(ctx, bobsReader, service.FeedSources{}, uuid.Nil, 10)
	if err != nil || len(feed.Posts) != 1 || feed.Posts[0].ID() != p.ID() {
		t.Fatalf("bob's posts in a feed = %+v, %v", feed.Posts, err)
	}

	// Whoever follows ann alone reads what bob wrote on her wall, once
	// even when following both.
	for _, followed := range []uuid.UUID{ann, bob} {
		if err = sources.Follow(ctx, annsReader, followed, false); err != nil {
			t.Fatal(err)
		}

		feed, err = svc.Feed(ctx, annsReader, service.FeedSources{}, uuid.Nil, 10)
		if got := texts(feed.Posts); err != nil || len(got) != 2 || got[0] != "привет, Аня" {
			t.Fatalf("the feed of ann's wall = %v, %v", got, err)
		}
	}

	// Unfollowed, they are read no more.
	if err = sources.Unfollow(ctx, bobsReader, bob); err != nil {
		t.Fatal(err)
	}
	if feed, err = svc.Feed(ctx, bobsReader, service.FeedSources{}, uuid.Nil, 10); err != nil || len(feed.Posts) != 0 {
		t.Fatalf("the feed after unfollowing = %+v, %v", feed.Posts, err)
	}
	if err = sources.Follow(ctx, ann, ann, false); !errors.Is(err, service.ErrSourceIsReader) {
		t.Fatalf("following oneself: %v", err)
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

	shared, err := svc.Repost(ctx, service.RepostInput{AuthorID: bob, PostID: original.ID(), Text: ""})
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
	passed, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: shared.ID(), Text: ""})
	if err != nil {
		t.Fatal(err)
	}
	if passed.Content().RepostOf() != original.ID() {
		t.Fatalf("a repost of an empty repost shares %v, want the original", passed.Content().RepostOf())
	}

	// A repost with a word of its own is shared itself.
	said, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: original.ID(), Text: "согласна"})
	if err != nil {
		t.Fatal(err)
	}
	ownWord, err := svc.Repost(ctx, service.RepostInput{AuthorID: bob, PostID: said.ID(), Text: ""})
	if err != nil || ownWord.Content().RepostOf() != said.ID() {
		t.Fatalf("sharing a repost with a word: %v, %v", ownWord, err)
	}

	got, _, err := svc.Get(ctx, uuid.Nil, original.ID())
	if err != nil || got.Counters().Reposts != 3 {
		t.Fatalf("original reposts = %+v, %v; want 3", got.Counters(), err)
	}

	if _, err = svc.Repost(ctx, service.RepostInput{AuthorID: ann, PostID: original.ID(), Text: "моё"}); err != nil {
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

	if _, err = svc.Repost(ctx, service.RepostInput{AuthorID: bob, PostID: uuid.New(), Text: ""}); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("sharing nothing: %v, want ErrNotFound", err)
	}

	// "в группу": for the community, signed; ann hears it from the club.
	club := uuid.New()
	inClub, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: original.ID(), WallID: club, FromWall: true, Signed: true})
	if err != nil || inClub.WallID() != club || inClub.AuthorID() != cat || !inClub.FromWall() || !inClub.Signed() {
		t.Fatalf("a repost in a club: %+v, %v", inClub, err)
	}
	last := notifier.notified[len(notifier.notified)-1]
	if last.ActorID != club || last.RecipientID != ann || last.PostID != inClub.ID() {
		t.Fatalf("notified %+v, want from the club", last)
	}

	// The words of a repost name ann, bob and cat herself: ann hears of
	// the repost, as the author of what is shared, and bob of his name.
	notifier.notified = nil
	named, err := svc.Repost(ctx, service.RepostInput{
		AuthorID: cat, PostID: original.ID(), Text: "@ann, @bob", Mentions: post.Mentions{ann, bob, cat},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNamed := []service.Event{
		{Kind: service.EventRepost, RecipientID: ann, ActorID: cat, PostID: named.ID()},
		{Kind: service.EventPostMention, RecipientID: bob, ActorID: cat, PostID: named.ID()},
	}
	if !slices.Equal(notifier.notified, wantNamed) {
		t.Fatalf("notified = %+v, want %+v", notifier.notified, wantNamed)
	}

	// For a community the name is said by the community.
	notifier.notified = nil
	forClub, err := svc.Repost(ctx, service.RepostInput{
		AuthorID: cat, PostID: original.ID(), Text: "@bob", WallID: club, FromWall: true, Mentions: post.Mentions{bob},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNamed = []service.Event{
		{Kind: service.EventRepost, RecipientID: ann, ActorID: club, PostID: forClub.ID()},
		{Kind: service.EventPostMention, RecipientID: bob, ActorID: club, PostID: forClub.ID()},
	}
	if !slices.Equal(notifier.notified, wantNamed) {
		t.Fatalf("notified for the club = %+v, want %+v", notifier.notified, wantNamed)
	}
}

func TestPostService_RepostsOfAPost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	photos := &fakePhotos{}
	svc := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Switches: store.Posts(),
		Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{},
	})

	ann, bob, cat, club := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	original := write(t, svc, ann, "оригинал")[0]

	// A repost says its own words with photos of its own, which media
	// keeps as those of any post.
	added := post.Photos{uuid.New(), uuid.New()}
	withPhotos, err := svc.Repost(ctx, service.RepostInput{AuthorID: bob, PostID: original.ID(), Photos: added})
	if err != nil || !slices.Equal(withPhotos.Content().Photos(), added) || withPhotos.Content().RepostOf() != original.ID() {
		t.Fatalf("a repost with photos = %+v, %v", withPhotos, err)
	}
	if len(photos.published) != 1 || photos.published[0].owner != bob || !slices.Equal(photos.published[0].photos, added) || photos.outsideTx != 0 {
		t.Fatalf("published = %+v, outside a transaction %d", photos.published, photos.outsideTx)
	}
	// Photos are words enough: shared, it is shared itself.
	if again, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: withPhotos.ID()}); err != nil || again.Content().RepostOf() != withPhotos.ID() {
		t.Fatalf("sharing a repost with photos: %+v, %v", again, err)
	}
	// An edit takes one away.
	if _, _, err = svc.Edit(ctx, bob, withPhotos.ID(), service.PostEdit{Photos: added[:1]}); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{added[1]}) {
		t.Fatalf("deleted = %+v, want the photo taken away", photos.deleted)
	}

	inClub, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: original.ID(), WallID: club, FromWall: true})
	if err != nil {
		t.Fatal(err)
	}
	own, err := svc.Repost(ctx, service.RepostInput{AuthorID: ann, PostID: original.ID(), Text: "моё"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.Repost(ctx, service.RepostInput{AuthorID: cat, PostID: original.ID(), Text: "в архив"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Archive(ctx, cat, hidden.ID()); err != nil {
		t.Fatal(err)
	}

	// Who shared it: newest first, a page at a time, without the archived
	// repost, which is counted still.
	page, err := svc.Reposts(ctx, uuid.Nil, original.ID(), uuid.Nil, 2)
	if err != nil || page.Total != 4 || len(page.Posts) != 2 || page.Posts[0].ID() != own.ID() || page.Posts[1].ID() != inClub.ID() ||
		page.NextCursor != inClub.ID() {
		t.Fatalf("Reposts = %+v, %v", page, err)
	}
	if page, err = svc.Reposts(ctx, uuid.Nil, original.ID(), page.NextCursor, 2); err != nil || len(page.Posts) != 1 ||
		page.Posts[0].ID() != withPhotos.ID() || page.NextCursor != uuid.Nil {
		t.Fatalf("Reposts, the rest = %+v, %v", page, err)
	}
	if _, err = svc.Reposts(ctx, uuid.Nil, uuid.New(), uuid.Nil, 2); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("reposts of no post: %v", err)
	}
	if _, err = svc.Reposts(ctx, uuid.Nil, hidden.ID(), uuid.Nil, 2); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("reposts of an archived post: %v", err)
	}

	// Deleted, the post takes back what its author heard of the reposts,
	// the archived one too; nobody heard of ann's own.
	if err = svc.Delete(ctx, ann, original.ID()); err != nil {
		t.Fatal(err)
	}

	want := []service.Event{
		{Kind: service.EventRepost, RecipientID: ann, ActorID: cat, PostID: hidden.ID()},
		{Kind: service.EventRepost, RecipientID: ann, ActorID: club, PostID: inClub.ID()},
		{Kind: service.EventRepost, RecipientID: ann, ActorID: bob, PostID: withPhotos.ID()},
	}
	if !slices.Equal(notifier.retracted, want) {
		t.Fatalf("retracted = %+v, want %+v", notifier.retracted, want)
	}
	if notifier.outsideTx != 0 {
		t.Fatalf("%d notifier calls outside a transaction", notifier.outsideTx)
	}

	// A post that speaks for a community has nobody to take anything from.
	notifier.retracted = nil
	forClub, err := svc.Create(ctx, service.CreateInput{WallID: club, AuthorID: ann, Content: textContent(t, "от группы"), ClubWall: true, FromWall: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Repost(ctx, service.RepostInput{AuthorID: bob, PostID: forClub.ID()}); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(ctx, ann, forClub.ID()); err != nil || len(notifier.retracted) != 0 {
		t.Fatalf("deleting a community's post: %v, retracted %+v", err, notifier.retracted)
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
	notifier := &fakeNotifier{}
	svc := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{}})
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

	if _, _, err = svc.Edit(ctx, stranger, p.ID(), service.PostEdit{Text: "чужое", Photos: nil}); !errors.Is(err, post.ErrCannotEdit) {
		t.Fatalf("a stranger edits: %v", err)
	}

	edited, liked, err := svc.Edit(ctx, author, p.ID(), service.PostEdit{Text: "стало", Photos: post.Photos{added, kept}})
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

	if _, _, err = svc.Edit(ctx, author, uuid.New(), service.PostEdit{Text: "нет", Photos: nil}); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("a missing post: %v", err)
	}
	if len(notifier.notified) != 0 {
		t.Fatalf("notified = %+v, want nobody: the edits name no one", notifier.notified)
	}

	// The edited text names a stranger and the author: the stranger hears.
	named := service.PostEdit{Text: "@stranger", Photos: post.Photos{kept}, Mentions: post.Mentions{stranger, author}}
	if _, _, err = svc.Edit(ctx, author, p.ID(), named); err != nil {
		t.Fatal(err)
	}
	want := []service.Event{{Kind: service.EventPostMention, RecipientID: stranger, ActorID: author, PostID: p.ID()}}
	if !slices.Equal(notifier.notified, want) {
		t.Fatalf("notified = %+v, want %+v", notifier.notified, want)
	}

	// On another's wall its owner heard of the post already.
	owner := uuid.New()
	guest, err := svc.Create(ctx, service.CreateInput{WallID: owner, AuthorID: author, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	notifier.notified = nil
	named = service.PostEdit{Text: "@owner, @stranger", Mentions: post.Mentions{owner, stranger}}
	if _, _, err = svc.Edit(ctx, author, guest.ID(), named); err != nil {
		t.Fatal(err)
	}
	want = []service.Event{{Kind: service.EventPostMention, RecipientID: stranger, ActorID: author, PostID: guest.ID()}}
	if !slices.Equal(notifier.notified, want) {
		t.Fatalf("notified on another's wall = %+v, want %+v", notifier.notified, want)
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

	page, err := svc.ListWall(ctx, uuid.Nil, owner, service.WallPick{Filter: post.WallAll}, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(page.Posts); !slices.Equal(got, []string{"первая", "четвёртая", "третья"}) || !page.Posts[0].Pinned() {
		t.Fatalf("first page = %v; want the pinned post on top of two others", got)
	}
	if page.Total != 4 {
		t.Fatalf("total = %d, the pinned post counts", page.Total)
	}

	next, err := svc.ListWall(ctx, uuid.Nil, owner, service.WallPick{Filter: post.WallAll}, page.NextCursor, 2)
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
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10); err != nil {
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
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10); err != nil {
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
	if page, err = svc.ListWall(ctx, uuid.Nil, owner, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10); err != nil || len(page.Posts) != 3 {
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
	signed, err := posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: admin, Content: content, ClubWall: true, FromWall: true, Signed: true})
	if err != nil || !signed.Signed() || !signed.FromWall() {
		t.Fatalf("Create signed = %+v, %v", signed, err)
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
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: member, PostID: news.ID(), Content: post.RestoreCommentContent("ура", nil)}); err != nil {
		t.Fatal(err)
	}
	if _, err = posts.Repost(ctx, service.RepostInput{AuthorID: member, PostID: news.ID(), Text: ""}); err != nil {
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
	strangers, err := comments.Create(ctx, service.CommentInput{AuthorID: stranger, PostID: byMember.ID(), Content: post.RestoreCommentContent("спам", nil)})
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Delete(ctx, member, strangers.ID()); err != nil {
		t.Fatalf("the author of the post deletes a comment: %v", err)
	}
	if strangers, err = comments.Create(ctx, service.CommentInput{AuthorID: stranger, PostID: news.ID(), Content: post.RestoreCommentContent("спам", nil)}); err != nil {
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
	svc, reads := newFeedService()
	club, admin, member, ann, cat := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
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
	annsOwn := mustCreate(service.CreateInput{WallID: ann, AuthorID: ann})

	// Following the admin shows their own posts, not those that speak for
	// the club: who wrote those is the club's secret.
	reads(ann, []uuid.UUID{admin})
	page, err := svc.Feed(ctx, ann, service.FeedSources{}, uuid.Nil, 20)
	if err != nil || len(page.Posts) != 2 || page.Posts[0].ID() != annsOwn.ID() || page.Posts[1].ID() != adminsOwn.ID() {
		t.Fatalf("feed of the admin = %v, %v", ids(page.Posts), err)
	}

	// Being in the club shows everything on its wall, and nothing else
	// its members wrote.
	reads(cat, nil, club)
	page, err = svc.Feed(ctx, cat, service.FeedSources{}, uuid.Nil, 20)
	if err != nil || len(page.Posts) != 2 || page.Posts[0].ID() != byMember.ID() || page.Posts[1].ID() != news.ID() {
		t.Fatalf("feed of the club = %v, %v", ids(page.Posts), err)
	}
}

func ids(posts []*post.Post) []uuid.UUID {
	out := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		out[i] = p.ID()
	}
	return out
}

func TestPostService_Likers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newPostService()
	ann := uuid.New()
	p := write(t, svc, ann, "привет")[0]

	var users []uuid.UUID
	for range 5 {
		u := uuid.New()
		users = append(users, u)
		if _, err := svc.Like(ctx, u, p.ID()); err != nil {
			t.Fatal(err)
		}
	}

	var (
		got    []uuid.UUID
		cursor *service.LikerCursor
	)
	for {
		page, err := svc.Likers(ctx, p.ID(), cursor, 2)
		if err != nil || page.Total != 5 {
			t.Fatalf("Likers = %+v, %v", page, err)
		}
		for _, l := range page.Likers {
			got = append(got, l.UserID)
		}
		if page.Next == nil {
			break
		}
		cursor = page.Next
	}

	// Each once across the pages; likes of one microsecond go by user.
	slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	slices.SortFunc(users, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(got, users) {
		t.Fatalf("likers %v, want %v", got, users)
	}

	if _, err := svc.Likers(ctx, uuid.New(), nil, 0); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("a missing post: %v", err)
	}
}
