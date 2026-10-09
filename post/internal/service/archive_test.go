package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// newWallServices are the post and comment services over one store, with
// admin running the community club.
func newWallServices(club, admin uuid.UUID) (*service.PostService, *service.CommentService) {
	store := inmem.NewStore()
	moderators := fakeModerators{{club, admin}: true}

	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: moderators, Switches: store.Posts(),
	})
	comments := service.NewCommentService(service.CommentDeps{
		Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: moderators,
	})

	return posts, comments
}

func TestPostService_Archive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	svc, comments := newWallServices(uuid.New(), uuid.New())

	posts := write(t, svc, ann, "раз", "два", "три")
	old := posts[0]
	if err := svc.Pin(ctx, ann, old.ID()); err != nil {
		t.Fatal(err)
	}

	if err := svc.Archive(ctx, bob, old.ID()); !errors.Is(err, post.ErrCannotArchive) {
		t.Fatalf("bob archives ann's post: %v", err)
	}
	if err := svc.Archive(ctx, ann, uuid.New()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("no post: %v", err)
	}
	for range 2 { // archived already is fine
		if err := svc.Archive(ctx, ann, old.ID()); err != nil {
			t.Fatalf("Archive: %v", err)
		}
	}

	// The wall and the feed no longer show it, pinned as it was.
	wall, err := svc.ListWall(ctx, bob, ann, service.WallPick{}, uuid.Nil, 10)
	if err != nil || wall.Total != 2 || !slices.Equal(texts(wall.Posts), []string{"три", "два"}) {
		t.Fatalf("the wall = %v, %d, %v", texts(wall.Posts), wall.Total, err)
	}
	if own, _ := svc.ListWall(ctx, ann, ann, service.WallPick{Filter: post.WallOwner}, uuid.Nil, 10); own.Total != 2 {
		t.Fatalf("ann's own posts: %d", own.Total)
	}
	if feed, _ := svc.Feed(ctx, bob, service.FeedSources{Authors: []uuid.UUID{ann}}, uuid.Nil, 10); len(feed.Posts) != 2 {
		t.Fatalf("the feed = %v", texts(feed.Posts))
	}

	// Only ann finds it: in her archive and by its address.
	archive, err := svc.ListWall(ctx, ann, ann, service.WallPick{Filter: post.WallArchive}, uuid.Nil, 10)
	if err != nil || archive.Total != 1 || archive.Posts[0].ID() != old.ID() || !archive.Posts[0].Archived() || archive.Posts[0].Pinned() {
		t.Fatalf("the archive = %+v, %v", archive.Posts, err)
	}
	for _, viewer := range []uuid.UUID{bob, uuid.Nil} {
		if _, err = svc.ListWall(ctx, viewer, ann, service.WallPick{Filter: post.WallArchive}, uuid.Nil, 10); !errors.Is(err, post.ErrCannotArchive) {
			t.Fatalf("%v reads ann's archive: %v", viewer, err)
		}
		if _, _, err = svc.Get(ctx, viewer, old.ID()); !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("%v gets an archived post: %v", viewer, err)
		}
	}
	if p, _, err := svc.Get(ctx, ann, old.ID()); err != nil || !p.Archived() {
		t.Fatalf("ann gets her archived post: %v", err)
	}
	if many, _ := svc.GetMany(ctx, []uuid.UUID{old.ID(), posts[1].ID()}); len(many) != 1 || many[0].ID() != posts[1].ID() {
		t.Fatalf("GetMany shows an archived post: %d", len(many))
	}

	// Nobody but ann comments what they cannot see.
	text := post.RestoreCommentContent("ау", nil)
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: old.ID(), Content: text}); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("bob comments an archived post: %v", err)
	}
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: old.ID(), Content: text}); err != nil {
		t.Fatalf("ann comments her archived post: %v", err)
	}

	if err = svc.Unarchive(ctx, bob, old.ID()); !errors.Is(err, post.ErrCannotArchive) {
		t.Fatalf("bob returns ann's post: %v", err)
	}
	if err = svc.Unarchive(ctx, ann, old.ID()); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	// It is back in its place, by its date, and not pinned again.
	if wall, _ = svc.ListWall(ctx, bob, ann, service.WallPick{}, uuid.Nil, 10); wall.Total != 3 || !slices.Equal(texts(wall.Posts), []string{"три", "два", "раз"}) {
		t.Fatalf("the wall after the return = %v", texts(wall.Posts))
	}
}

func TestPostService_ArchiveOfClub(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, member := uuid.New(), uuid.New(), uuid.New()
	svc, _ := newWallServices(club, admin)

	p, err := svc.Create(ctx, service.CreateInput{WallID: club, AuthorID: member, Content: textContent(t, "привет"), ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}

	// The author of a post on a community's wall does not archive it.
	if err = svc.Archive(ctx, member, p.ID()); !errors.Is(err, post.ErrCannotArchive) {
		t.Fatalf("a member archives: %v", err)
	}
	if err = svc.Archive(ctx, admin, p.ID()); err != nil {
		t.Fatalf("an admin archives: %v", err)
	}
	if page, err := svc.ListWall(ctx, admin, club, service.WallPick{Filter: post.WallArchive}, uuid.Nil, 10); err != nil || page.Total != 1 {
		t.Fatalf("the admin's archive = %d, %v", page.Total, err)
	}
	if _, err = svc.ListWall(ctx, member, club, service.WallPick{Filter: post.WallArchive}, uuid.Nil, 10); !errors.Is(err, post.ErrCannotArchive) {
		t.Fatalf("a member reads the archive: %v", err)
	}
	if _, _, err = svc.Get(ctx, admin, p.ID()); err != nil {
		t.Fatalf("an admin gets the archived post: %v", err)
	}
	if _, _, err = svc.Get(ctx, member, p.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("its author gets the archived post: %v", err)
	}
}

func TestPostService_SearchWall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	svc, _ := newWallServices(uuid.New(), uuid.New())

	posts := write(t, svc, ann, "Еду на море", "горы", "МОРЕ волнуется", "скидка 50%")
	if err := svc.Pin(ctx, ann, posts[0].ID()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Archive(ctx, ann, posts[2].ID()); err != nil {
		t.Fatal(err)
	}

	search := func(q string, filter post.WallFilter) service.Page {
		t.Helper()

		query, err := post.NewWallQuery(q)
		if err != nil {
			t.Fatal(err)
		}

		page, err := svc.ListWall(ctx, ann, ann, service.WallPick{Filter: filter, Query: query}, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}

		return page
	}

	// The pinned post is found once, in its place; the archived one is not.
	if page := search("море", post.WallAll); page.Total != 1 || !slices.Equal(texts(page.Posts), []string{"Еду на море"}) {
		t.Fatalf("море = %v, %d", texts(page.Posts), page.Total)
	}
	if page := search("море", post.WallArchive); page.Total != 1 || page.Posts[0].ID() != posts[2].ID() {
		t.Fatalf("море in the archive = %v", texts(page.Posts))
	}
	if page := search("%", post.WallAll); page.Total != 1 || page.Posts[0].ID() != posts[3].ID() {
		t.Fatalf("%% = %v", texts(page.Posts))
	}
	if page := search("снег", post.WallAll); page.Total != 0 || len(page.Posts) != 0 {
		t.Fatalf("снег = %v", texts(page.Posts))
	}
}

func TestPostService_CloseComments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	svc, comments := newWallServices(uuid.New(), uuid.New())

	// Bob's post on ann's wall: ann closes its comments, bob does not.
	p, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: textContent(t, "привет")})
	if err != nil {
		t.Fatal(err)
	}
	text := post.RestoreCommentContent("ответ", nil)
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: p.ID(), Content: text}); err != nil {
		t.Fatal(err)
	}

	if err = svc.CloseComments(ctx, bob, p.ID()); !errors.Is(err, post.ErrCannotCloseComments) {
		t.Fatalf("the author closes the comments on ann's wall: %v", err)
	}
	if err = svc.CloseComments(ctx, ann, p.ID()); err != nil {
		t.Fatalf("CloseComments: %v", err)
	}

	got, _, err := svc.Get(ctx, bob, p.ID())
	if err != nil || !got.CommentsClosed() || got.Counters().Comments != 1 {
		t.Fatalf("the post = %+v, %v; closed, with the comment written before", got, err)
	}
	for _, author := range []uuid.UUID{ann, bob} {
		if _, err = comments.Create(ctx, service.CommentInput{AuthorID: author, PostID: p.ID(), Content: text}); !errors.Is(err, post.ErrCommentsClosed) {
			t.Fatalf("%v comments a closed post: %v", author, err)
		}
	}

	if err = svc.OpenComments(ctx, bob, p.ID()); !errors.Is(err, post.ErrCannotCloseComments) {
		t.Fatalf("the author opens them: %v", err)
	}
	if err = svc.OpenComments(ctx, ann, p.ID()); err != nil {
		t.Fatalf("OpenComments: %v", err)
	}
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: p.ID(), Content: text}); err != nil {
		t.Fatalf("a comment after they are opened: %v", err)
	}
}
