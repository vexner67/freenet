package postgres_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

// newPool connects to the database of the integration tests, which needs
// migrations applied, e.g. TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// reader is someone new who reads people and, after them, clubs in the
// news.
func reader(t *testing.T, people []uuid.UUID, clubs ...uuid.UUID) service.FeedSources {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := postgres.NewSourceRepository(newPool(t))
	id := uuid.New()

	for _, source := range people {
		if err := repo.Add(ctx, id, source, false); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	for _, source := range clubs {
		if err := repo.Add(ctx, id, source, true); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	return service.FeedSources{Reader: id}
}

func TestPostRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	author := uuid.New()

	var saved []*post.Post
	for _, text := range []string{"раз", "два", "три"} {
		p, err := post.New(author, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
		saved = append(saved, p)
	}

	got, err := repo.Get(ctx, saved[0].ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, saved[0]) {
		t.Fatalf("round trip: got %+v, want %+v", got, saved[0])
	}

	photos, err := post.NewPhotos([]uuid.UUID{uuid.New(), uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	pictured, err := post.New(author, author, post.RestoreContent("", photos, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, pictured); err != nil {
		t.Fatalf("Save a post of photos only: %v", err)
	}
	if got, err := repo.Get(ctx, pictured.ID()); err != nil || !reflect.DeepEqual(got, pictured) {
		t.Fatalf("photos round trip: got %+v, %v; want %+v", got, err, pictured)
	}
	if err = repo.Delete(ctx, pictured.ID()); err != nil {
		t.Fatal(err)
	}

	many, err := repo.GetMany(ctx, []uuid.UUID{saved[2].ID(), uuid.New(), saved[0].ID()})
	if err != nil || len(many) != 2 {
		t.Fatalf("GetMany = %v, %v", many, err)
	}

	page, err := repo.ListByWall(ctx, author, service.WallPick{Filter: post.WallAll}, uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Content().Text() != "три" || page[1].Content().Text() != "два" {
		t.Fatalf("first page: %v", page)
	}

	rest, err := repo.ListByWall(ctx, author, service.WallPick{Filter: post.WallAll}, page[1].ID(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0].Content().Text() != "раз" {
		t.Fatalf("after cursor: %v", rest)
	}

	other := uuid.New()
	theirs, err := post.New(other, other, post.RestoreContent("чужая", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, theirs); err != nil {
		t.Fatal(err)
	}

	// other writes on author's wall: the post is on that wall and among
	// other's posts.
	guest, err := post.New(author, other, post.RestoreContent("в гостях", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, guest); err != nil {
		t.Fatal(err)
	}

	visit, err := repo.Get(ctx, guest.ID())
	if err != nil || visit.WallID() != author || visit.AuthorID() != other {
		t.Fatalf("Get(guest) = %+v, %v", visit, err)
	}

	wall, err := repo.ListByWall(ctx, author, service.WallPick{Filter: post.WallAll}, uuid.Nil, 10)
	if err != nil || len(wall) != 4 || wall[0].ID() != guest.ID() || wall[3].Content().Text() != "раз" {
		t.Fatalf("ListByWall with a guest post = %v, %v", wall, err)
	}

	if n, err := repo.CountByWall(ctx, author, service.WallPick{Filter: post.WallAll}); err != nil || n != 4 {
		t.Fatalf("CountByWall = %d, %v", n, err)
	}

	// The tabs of the wall: the owner's posts and the others'.
	own, err := repo.ListByWall(ctx, author, service.WallPick{Filter: post.WallOwner}, uuid.Nil, 10)
	if err != nil || len(own) != 3 || own[0].Content().Text() != "три" {
		t.Fatalf("the owner's = %v, %v", own, err)
	}
	others, err := repo.ListByWall(ctx, author, service.WallPick{Filter: post.WallOthers}, uuid.Nil, 10)
	if err != nil || len(others) != 1 || others[0].ID() != guest.ID() {
		t.Fatalf("the others' = %v, %v", others, err)
	}
	for filter, want := range map[post.WallFilter]int{post.WallOwner: 3, post.WallOthers: 1} {
		if n, err := repo.CountByWall(ctx, author, service.WallPick{Filter: filter}); err != nil || n != want {
			t.Fatalf("CountByWall(%q) = %d, %v", filter, n, err)
		}
	}

	byOther, err := repo.ListFeed(ctx, reader(t, []uuid.UUID{other}), uuid.Nil, 10)
	if err != nil || len(byOther) != 2 || byOther[0].ID() != guest.ID() || byOther[1].ID() != theirs.ID() {
		t.Fatalf("ListFeed(other) = %v, %v", byOther, err)
	}

	feed, err := repo.ListFeed(ctx, reader(t, []uuid.UUID{author, other}), uuid.Nil, 10)
	if err != nil || len(feed) != 5 || feed[0].ID() != guest.ID() || feed[4].Content().Text() != "раз" {
		t.Fatalf("ListFeed = %v, %v", feed, err)
	}

	// The walls of a feed bring what others wrote there, once.
	walls, err := repo.ListFeed(ctx, reader(t, []uuid.UUID{author}), uuid.Nil, 10)
	if err != nil || len(walls) != 4 || walls[0].ID() != guest.ID() {
		t.Fatalf("ListFeed with the wall = %v, %v", walls, err)
	}

	if err = repo.Delete(ctx, saved[0].ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = repo.Delete(ctx, saved[0].ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("second Delete: %v", err)
	}
	if _, err = repo.Get(ctx, saved[0].ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
}

func TestPostRepository_ClubWalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := postgres.NewPostRepository(newPool(t))
	club, admin, member := uuid.New(), uuid.New(), uuid.New()
	content := post.RestoreContent("x", nil, uuid.Nil)

	news, err := post.NewFromWall(club, admin, content, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	byMember, err := post.New(club, member, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	adminsOwn, err := post.New(admin, admin, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*post.Post{news, byMember, adminsOwn} {
		if err = repo.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := repo.Get(ctx, news.ID())
	if err != nil || !got.FromWall() || !got.Signed() || got.AuthorID() != admin {
		t.Fatalf("Get = %+v, %v; it speaks for the club", got, err)
	}
	if got, _ = repo.Get(ctx, byMember.ID()); got.FromWall() {
		t.Fatal("the member's post speaks for the member")
	}

	byAuthor, err := repo.ListFeed(ctx, reader(t, []uuid.UUID{admin}), uuid.Nil, 10)
	if err != nil || len(byAuthor) != 1 || byAuthor[0].ID() != adminsOwn.ID() {
		t.Fatalf("feed of the admin = %v, %v; not the club's post", byAuthor, err)
	}

	both, err := repo.ListFeed(ctx, reader(t, []uuid.UUID{admin}, club), uuid.Nil, 10)
	if err != nil || len(both) != 3 || both[0].ID() != adminsOwn.ID() || both[1].ID() != byMember.ID() || both[2].ID() != news.ID() {
		t.Fatalf("feed of the admin and the club = %v, %v", both, err)
	}

	if older, err := repo.ListFeed(ctx, reader(t, nil, club), byMember.ID(), 10); err != nil || len(older) != 1 || older[0].ID() != news.ID() {
		t.Fatalf("feed after a cursor = %v, %v", older, err)
	}

	// The posts of these walls in a feed, among those of other tests.
	here := func(sources service.FeedSources) []uuid.UUID {
		t.Helper()

		feed, err := repo.ListFeed(ctx, sources, uuid.Nil, 1000)
		if err != nil {
			t.Fatalf("ListFeed(%+v): %v", sources, err)
		}

		var found []uuid.UUID
		for _, p := range feed {
			if p.WallID() == club || p.WallID() == admin {
				found = append(found, p.ID())
			}
		}

		return found
	}

	// "Все новости": the posts of every wall; without "Показывать посты на
	// чужих стенах" only what the walls say themselves, not what guests
	// wrote; and never the walls the reader ignores.
	if got := here(service.FeedSources{Everyone: true}); !slices.Equal(got, []uuid.UUID{adminsOwn.ID(), byMember.ID(), news.ID()}) {
		t.Fatalf("the feed of everyone = %v; want all three", got)
	}
	if got := here(service.FeedSources{Everyone: true, OwnOnly: true}); !slices.Equal(got, []uuid.UUID{adminsOwn.ID(), news.ID()}) {
		t.Fatalf("the feed of everyone, own only = %v; want the admin's own post and the club's", got)
	}
	if got := here(service.FeedSources{Everyone: true, Ignored: []uuid.UUID{club}}); !slices.Equal(got, []uuid.UUID{adminsOwn.ID()}) {
		t.Fatalf("the feed of everyone without the club = %v", got)
	}

	mine := reader(t, []uuid.UUID{admin, member}, club)
	mine.OwnOnly = true
	if got := here(mine); !slices.Equal(got, []uuid.UUID{adminsOwn.ID(), news.ID()}) {
		t.Fatalf("the feed, own only = %v", got)
	}
	mine.OwnOnly, mine.Ignored = false, []uuid.UUID{club}
	if got := here(mine); !slices.Equal(got, []uuid.UUID{adminsOwn.ID()}) {
		t.Fatalf("the feed without the club = %v; the member's post there goes too", got)
	}

	photo, err := post.NewContent("", post.Photos{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	newPhoto, err := post.NewAvatarUpdate(club, admin, photo, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, newPhoto); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, err := repo.Get(ctx, newPhoto.ID()); err != nil || !got.AvatarUpdate() || !got.FromWall() {
		t.Fatalf("the club's new photo = %+v, %v", got, err)
	}
}

func TestSourceRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := newPool(t)
	posts, sources := postgres.NewPostRepository(pool), postgres.NewSourceRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	for _, author := range []uuid.UUID{ann, bob} {
		p, err := post.New(author, author, post.RestoreContent("x", nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	feed := func(reader uuid.UUID) int {
		t.Helper()

		page, err := posts.ListFeed(ctx, service.FeedSources{Reader: reader}, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("ListFeed: %v", err)
		}

		return len(page)
	}

	// Nobody followed: only one's own posts; nobody at all reads nothing.
	if got := feed(cat); got != 0 {
		t.Fatalf("cat reads %d posts before following", got)
	}
	if got := feed(ann); got != 1 {
		t.Fatalf("ann reads %d posts, want her own", got)
	}
	if got := feed(uuid.Nil); got != 0 {
		t.Fatalf("nobody reads %d posts", got)
	}

	// Told twice, a follow is one follow.
	for range 2 {
		for _, source := range []uuid.UUID{ann, bob} {
			if err := sources.Add(ctx, cat, source, false); err != nil {
				t.Fatalf("Add: %v", err)
			}
		}
	}
	if err := sources.Add(ctx, ann, bob, false); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := feed(cat); got != 2 {
		t.Fatalf("cat reads %d posts, want ann's and bob's", got)
	}

	for range 2 {
		if err := sources.Remove(ctx, cat, bob); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}
	if got := feed(cat); got != 1 {
		t.Fatalf("cat reads %d posts after unfollowing bob", got)
	}

	// Erased, ann is read by nobody and reads nobody.
	if err := sources.Forget(ctx, ann); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if got := feed(cat); got != 0 {
		t.Fatalf("cat reads %d posts of the erased", got)
	}
	if got := feed(ann); got != 1 {
		t.Fatalf("the erased reads %d posts, want only her own", got)
	}
}
