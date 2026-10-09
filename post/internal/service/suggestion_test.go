package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestSuggestionService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	photos := &fakePhotos{}
	svc := service.NewSuggestionService(service.SuggestionDeps{
		Suggestions: inmem.NewSuggestionRepository(), Posts: store.Posts(), Tx: fakeTx{}, Notifier: notifier, Photos: photos,
		Moderators: fakeModerators{{club, admin}: true},
	})
	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos,
		Moderators: fakeModerators{},
	})

	photo := uuid.New()
	withPhoto, err := post.NewContent("Котик нашёлся", post.Photos{photo})
	if err != nil {
		t.Fatal(err)
	}
	words, _ := post.NewContent("Встреча в субботу", nil)

	first, err := svc.Suggest(ctx, club, ann, withPhoto)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos.published) != 1 || photos.published[0].owner != ann {
		t.Fatalf("the photos are kept at once: %+v", photos.published)
	}
	if n := notifier.notified; len(n) != 1 || n[0].Kind != service.EventSuggestedPost || n[0].RecipientID != admin || n[0].ActorID != club || n[0].PostID != first.ID() || notifier.outsideTx != 0 {
		t.Fatalf("the admin hears from the club: %+v", n)
	}
	notifier.notified = nil

	second, _ := svc.Suggest(ctx, club, ann, words)
	bobs, _ := svc.Suggest(ctx, club, bob, words)
	notifier.notified = nil

	// The wall shows none of them.
	if wall, err := posts.ListWall(ctx, uuid.Nil, club, post.WallAll, uuid.Nil, 10); err != nil || len(wall.Posts) != 0 || wall.Total != 0 {
		t.Fatalf("the wall = %+v, %v", wall, err)
	}

	mine, err := svc.List(ctx, ann, club, true, uuid.Nil, 1)
	if err != nil || len(mine.Suggestions) != 1 || mine.Suggestions[0].ID() != second.ID() || mine.Total != 2 || mine.NextCursor != second.ID() {
		t.Fatalf("ann's = %+v, %v", mine, err)
	}
	if _, err = svc.List(ctx, ann, club, false, uuid.Nil, 10); !errors.Is(err, post.ErrCannotSeeAll) {
		t.Fatalf("ann sees all: %v", err)
	}
	all, err := svc.List(ctx, admin, club, false, uuid.Nil, 10)
	if err != nil || all.Total != 3 || all.Suggestions[0].ID() != bobs.ID() {
		t.Fatalf("all = %+v, %v", all, err)
	}

	if _, err = svc.Publish(ctx, ann, first.ID(), "сама", false); !errors.Is(err, post.ErrCannotPublish) {
		t.Fatalf("the author publishes: %v", err)
	}
	published, err := svc.Publish(ctx, admin, first.ID(), "Котик нашёлся!", true)
	if err != nil {
		t.Fatal(err)
	}
	if !published.FromWall() || !published.Signed() || published.AuthorID() != ann || published.WallID() != club || len(published.Content().Photos()) != 1 {
		t.Fatalf("published = %+v", published)
	}
	if len(photos.published) != 1 {
		t.Fatalf("the photos are published twice: %+v", photos.published)
	}
	if n := notifier.notified; len(n) != 1 || n[0].Kind != service.EventPostAccepted || n[0].RecipientID != ann || n[0].ActorID != club || n[0].PostID != published.ID() {
		t.Fatalf("notified = %+v", n)
	}
	if f := notifier.forgotten; len(f) != 1 || f[0] != first.ID() {
		t.Fatalf("the admins' notifications about the suggestion stay: %v", f)
	}
	if wall, _ := posts.ListWall(ctx, uuid.Nil, club, post.WallAll, uuid.Nil, 10); len(wall.Posts) != 1 || wall.Posts[0].ID() != published.ID() {
		t.Fatalf("the wall after publishing = %+v", wall)
	}
	if _, err = svc.Publish(ctx, admin, first.ID(), "ещё раз", false); !errors.Is(err, post.ErrSuggestionNotFound) {
		t.Fatalf("published twice: %v", err)
	}

	// Bob withdraws nobody's but his own; the admin declines any.
	if err = svc.Withdraw(ctx, bob, second.ID()); !errors.Is(err, post.ErrCannotWithdraw) {
		t.Fatalf("bob withdraws ann's: %v", err)
	}
	if err = svc.Withdraw(ctx, bob, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.Withdraw(ctx, admin, second.ID()); err != nil {
		t.Fatal(err)
	}
	if left, _ := svc.List(ctx, admin, club, false, uuid.Nil, 10); left.Total != 0 {
		t.Fatalf("left = %+v", left)
	}
	if f := notifier.forgotten; len(f) != 3 || f[1] != bobs.ID() || f[2] != second.ID() {
		t.Fatalf("withdrawn and declined take their notifications: %v", f)
	}
	if err = svc.Withdraw(ctx, admin, second.ID()); !errors.Is(err, post.ErrSuggestionNotFound) {
		t.Fatalf("declined twice: %v", err)
	}

	// An admin's own suggestion, from before they ran the club, tells
	// nobody; a suggestion of words only needs words.
	own, _ := svc.Suggest(ctx, club, admin, words)
	if len(notifier.notified) != 1 {
		t.Fatalf("the admin hears about their own: %+v", notifier.notified)
	}
	if _, err = svc.Publish(ctx, admin, own.ID(), "", false); !errors.Is(err, post.ErrTextEmpty) {
		t.Fatalf("published empty: %v", err)
	}
	if _, err = svc.Publish(ctx, admin, own.ID(), "Встреча", false); err != nil || len(notifier.notified) != 1 {
		t.Fatalf("an admin's own = %v, notified %d", err, len(notifier.notified))
	}
}

func TestSuggestionService_WithdrawPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, ann := uuid.New(), uuid.New()
	photos := &fakePhotos{}
	svc := service.NewSuggestionService(service.SuggestionDeps{
		Suggestions: inmem.NewSuggestionRepository(), Posts: inmem.NewStore().Posts(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: photos,
		Moderators: fakeModerators{},
	})

	photo := uuid.New()
	content, _ := post.NewContent("", post.Photos{photo})
	s, err := svc.Suggest(ctx, club, ann, content)
	if err != nil {
		t.Fatal(err)
	}

	if err = svc.Withdraw(ctx, ann, s.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != ann || photos.deleted[0].photos[0] != photo || photos.outsideTx != 0 {
		t.Fatalf("deleted = %+v", photos.deleted)
	}
}
