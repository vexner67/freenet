package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type postponedFixture struct {
	svc      *service.PostponedService
	posts    *service.PostService
	albums   *service.AlbumService
	notifier *fakeNotifier
	photos   *fakePhotos
	now      time.Time
}

func newPostponedFixture(t *testing.T, moderators fakeModerators) *postponedFixture {
	t.Helper()

	f := &postponedFixture{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC), notifier: &fakeNotifier{}}
	albums, photos := newAlbumService(t, fakeModerators{})
	f.albums, f.photos = albums, photos
	store := inmem.NewStore()
	f.posts = service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: f.notifier, Photos: photos, Moderators: moderators, WallPhotos: albums,
	})
	f.svc = service.NewPostponedService(service.PostponedDeps{
		Postponed: inmem.NewPostponedRepository(), Poster: f.posts, Tx: fakeTx{}, Photos: photos, Moderators: moderators,
		Now: func() time.Time { return f.now },
	})

	return f
}

func (f *postponedFixture) wall(t *testing.T, wallID uuid.UUID) []*post.Post {
	t.Helper()

	page, err := f.posts.ListWall(context.Background(), uuid.Nil, wallID, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}

	return page.Posts
}

func TestPostponedService_PublishDue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	f := newPostponedFixture(t, fakeModerators{})

	photo := uuid.New()
	content, _ := post.NewContent("Завтра в 10", post.Photos{photo})
	later, _ := f.svc.Postpone(ctx, post.PostponedDraft{
		WallID: ann, AuthorID: ann, Content: content, PublishAt: f.now.Add(2 * time.Hour), NSFW: true, Mentions: post.Mentions{bob},
	})
	words, _ := post.NewContent("Через час", nil)
	sooner, err := f.svc.Postpone(ctx, post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.photos.published) != 1 || f.photos.published[0].owner != ann || f.photos.outsideTx != 0 {
		t.Fatalf("the photos are kept at once, in the transaction: %+v", f.photos.published)
	}

	// Nobody sees them on the wall, and only the owner in the list.
	if wall := f.wall(t, ann); len(wall) != 0 {
		t.Fatalf("the wall shows %d postponed posts", len(wall))
	}
	list, err := f.svc.List(ctx, ann, ann)
	if err != nil || len(list) != 2 || list[0].ID() != sooner.ID() || list[1].ID() != later.ID() {
		t.Fatalf("the owner's list = %v, %v", list, err)
	}
	if _, err = f.svc.List(ctx, bob, ann); !errors.Is(err, post.ErrCannotSeePostponed) {
		t.Fatalf("bob's list: %v", err)
	}
	if _, err = f.svc.Get(ctx, bob, later.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("bob gets it: %v", err)
	}

	// Nothing is due yet.
	if n, err := f.svc.PublishDue(ctx); n != 0 || err != nil {
		t.Fatalf("PublishDue before time = %d, %v", n, err)
	}

	f.now = f.now.Add(90 * time.Minute)
	if n, err := f.svc.PublishDue(ctx); n != 1 || err != nil {
		t.Fatalf("PublishDue = %d, %v", n, err)
	}
	wall := f.wall(t, ann)
	if len(wall) != 1 || wall[0].Content().Text() != "Через час" || !wall[0].CreatedAt().Equal(f.now) || wall[0].ID() == sooner.ID() {
		t.Fatalf("the wall = %+v", wall)
	}
	if _, err = f.svc.Get(ctx, ann, sooner.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("the published one still waits: %v", err)
	}

	f.now = f.now.Add(time.Hour)
	if n, err := f.svc.PublishDue(ctx); n != 1 || err != nil {
		t.Fatalf("PublishDue = %d, %v", n, err)
	}
	wall = f.wall(t, ann)
	if len(wall) != 2 || !wall[0].NSFW() || len(wall[0].Content().Photos()) != 1 {
		t.Fatalf("the wall = %+v", wall)
	}

	// The one it names hears of it, and its photo goes to "Фотографии со
	// стены" as a copy; media is not told to keep it twice.
	if n := f.notifier.notified; len(n) != 1 || n[0].Kind != service.EventPostMention || n[0].RecipientID != bob || n[0].PostID != wall[0].ID() {
		t.Fatalf("notified = %+v", n)
	}
	if len(f.photos.published) != 1 || len(f.photos.copied) != 1 {
		t.Fatalf("published %d, copied %d", len(f.photos.published), len(f.photos.copied))
	}
	if albums, _ := f.albums.List(ctx, ann, uuid.Nil, 0); albums.Total != 1 {
		t.Fatalf("albums = %d", albums.Total)
	}
	if f.notifier.outsideTx != 0 {
		t.Fatal("notified outside the transaction")
	}
}

func TestPostponedService_Community(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := newPostponedFixture(t, fakeModerators{{club, admin}: true, {club, ann}: true})

	words, _ := post.NewContent("Встреча в субботу", nil)
	p, err := f.svc.Postpone(ctx, post.PostponedDraft{
		WallID: club, AuthorID: ann, Content: words, FromWall: true, Signed: true, PublishAt: f.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if list, err := f.svc.List(ctx, admin, club); err != nil || len(list) != 1 {
		t.Fatalf("the admin's list = %v, %v", list, err)
	}
	if _, err = f.svc.List(ctx, bob, club); !errors.Is(err, post.ErrCannotSeePostponed) {
		t.Fatalf("bob's list: %v", err)
	}

	// Another admin edits it.
	edited, err := f.svc.Edit(ctx, admin, p.ID(), post.PostponedEdit{Text: "Встреча в воскресенье", PublishAt: f.now.Add(3 * time.Hour)})
	if err != nil || edited.Content().Text() != "Встреча в воскресенье" {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}
	if _, err = f.svc.Edit(ctx, bob, p.ID(), post.PostponedEdit{Text: "x", PublishAt: f.now.Add(time.Hour)}); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("bob edits: %v", err)
	}
	if _, err = f.svc.Edit(ctx, admin, p.ID(), post.PostponedEdit{Text: "x", PublishAt: f.now}); !errors.Is(err, post.ErrPublishTime) {
		t.Fatalf("edited to now: %v", err)
	}

	// Published at once: it speaks for the community, signed, dated now.
	published, err := f.svc.Publish(ctx, admin, p.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !published.FromWall() || !published.Signed() || published.AuthorID() != ann || published.Content().Text() != "Встреча в воскресенье" ||
		!published.CreatedAt().Equal(f.now) {
		t.Fatalf("published = %+v", published)
	}
	if _, err = f.svc.Publish(ctx, admin, p.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("published twice: %v", err)
	}

	// The wall's owner is nobody to notify.
	if len(f.notifier.notified) != 0 {
		t.Fatalf("notified = %+v", f.notifier.notified)
	}
}

func TestPostponedService_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := newPostponedFixture(t, fakeModerators{{club, admin}: true})

	photo := uuid.New()
	content, _ := post.NewContent("", post.Photos{photo})
	// Ann was an admin when she postponed it; now she is not.
	p, err := f.svc.Postpone(ctx, post.PostponedDraft{WallID: club, AuthorID: ann, Content: content, FromWall: true, PublishAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = f.svc.Publish(ctx, ann, p.ID()); !errors.Is(err, post.ErrCannotPostpone) {
		t.Fatalf("a former admin publishes for the community: %v", err)
	}
	if err = f.svc.Delete(ctx, bob, p.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("bob deletes: %v", err)
	}

	// Still, she takes back what she postponed, with its photo.
	if err = f.svc.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}
	if len(f.photos.deleted) != 1 || f.photos.deleted[0].owner != ann || f.photos.deleted[0].photos[0] != photo {
		t.Fatalf("deleted photos = %+v", f.photos.deleted)
	}
	if err = f.svc.Delete(ctx, admin, p.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}

	// A post due after it was deleted is not published.
	f.now = f.now.Add(2 * time.Hour)
	if n, err := f.svc.PublishDue(ctx); n != 0 || err != nil {
		t.Fatalf("PublishDue = %d, %v", n, err)
	}
	if wall := f.wall(t, club); len(wall) != 0 {
		t.Fatalf("the wall = %+v", wall)
	}
}

func TestPostponedService_Limit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	f := newPostponedFixture(t, fakeModerators{})
	words, _ := post.NewContent("ещё", nil)

	for i := range post.MaxPostponed {
		d := post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: f.now.Add(time.Duration(i+1) * time.Minute)}
		if _, err := f.svc.Postpone(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	d := post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: f.now.Add(time.Hour)}
	if _, err := f.svc.Postpone(ctx, d); !errors.Is(err, post.ErrTooManyPostponed) {
		t.Fatalf("one too many: %v", err)
	}

	// A round publishes at most MaxDue; the rest wait for the next.
	f.now = f.now.Add(24 * time.Hour)
	if n, err := f.svc.PublishDue(ctx); n != min(post.MaxPostponed, service.MaxDue) || err != nil {
		t.Fatalf("PublishDue = %d, %v", n, err)
	}
}

func TestPurgeService_Postponed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	repo := inmem.NewPostponedRepository()
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Sources: store.Sources(),
		Comments: store.Comments(), Suggestions: inmem.NewSuggestionRepository(), Topics: inmem.NewTopicRepository(),
		Albums: inmem.NewAlbumRepository(), Notes: inmem.NewNoteRepository(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{},
		Postponed: repo,
	})
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	words, _ := post.NewContent("потом", nil)

	postpone := func(d post.PostponedDraft) *post.Postponed {
		t.Helper()
		d.Content, d.PublishAt = words, now.Add(time.Hour)
		p, err := post.NewPostponed(d, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	own := postpone(post.PostponedDraft{WallID: ann, AuthorID: ann})
	forClub := postpone(post.PostponedDraft{WallID: club, AuthorID: ann, FromWall: true})
	bobs := postpone(post.PostponedDraft{WallID: club, AuthorID: bob, FromWall: true})

	if err := purge.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}

	for _, p := range []*post.Postponed{own, forClub} {
		if _, err := repo.Get(ctx, p.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
			t.Fatalf("ann's postponed post stays: %v", err)
		}
	}
	if _, err := repo.Get(ctx, bobs.ID()); err != nil {
		t.Fatalf("bob's: %v", err)
	}
}
