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

func TestPurgeService(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	photos := &fakePhotos{}
	posts := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos})
	comments := service.NewCommentService(store.Posts(), store.Comments(), fakeTx{}, notifier)
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), Comments: store.Comments(), Tx: fakeTx{}, Notifier: notifier, Photos: photos,
	})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	annsPhoto, bobsPhoto := uuid.New(), uuid.New()
	annsContent, err := post.NewContent("запись Анны", post.Photos{annsPhoto})
	if err != nil {
		t.Fatal(err)
	}
	annsPost, err := posts.Create(ctx, ann, ann, annsContent)
	if err != nil {
		t.Fatal(err)
	}
	bobsPost := write(t, posts, bob, "запись Петра")[0]
	content, err := post.NewContent("Пётр на стене Анны", post.Photos{bobsPhoto})
	if err != nil {
		t.Fatal(err)
	}
	onAnnsWall, err := posts.Create(ctx, ann, bob, content)
	if err != nil {
		t.Fatal(err)
	}
	annsRepost, err := posts.Repost(ctx, ann, bobsPost.ID(), "")
	if err != nil {
		t.Fatal(err)
	}

	// Ann likes and comments on bob's post; bob likes his own too.
	for _, who := range []uuid.UUID{ann, bob} {
		if _, err = posts.Like(ctx, who, bobsPost.ID()); err != nil {
			t.Fatal(err)
		}
	}
	text, err := post.NewCommentText("привет")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = comments.Create(ctx, ann, bobsPost.ID(), text); err != nil {
		t.Fatal(err)
	}

	notifier.forgotten = nil
	if err = purge.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err = purge.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	for _, gone := range []uuid.UUID{annsPost.ID(), onAnnsWall.ID(), annsRepost.ID()} {
		if _, _, err = posts.Get(ctx, uuid.Nil, gone); !errors.Is(err, post.ErrNotFound) {
			t.Errorf("post %s is still there: %v", gone, err)
		}
	}

	kept, _, err := posts.Get(ctx, uuid.Nil, bobsPost.ID())
	if err != nil {
		t.Fatal(err)
	}
	if c := kept.Counters(); c.Likes != 1 || c.Comments != 0 || c.Reposts != 0 {
		t.Errorf("bob's post counters = %+v, want bob's like only", c)
	}

	want := []uuid.UUID{annsPost.ID(), onAnnsWall.ID(), annsRepost.ID()}
	if len(notifier.forgotten) != len(want) {
		t.Errorf("forgotten = %v, want the 3 deleted posts", notifier.forgotten)
	}
	for _, id := range want {
		if !slices.Contains(notifier.forgotten, id) {
			t.Errorf("the notifications of %s were not forgotten", id)
		}
	}
	if notifier.outsideTx != 0 {
		t.Errorf("%d notifications recorded outside the transaction", notifier.outsideTx)
	}

	// Bob's photo goes with his post on ann's wall; ann's own media erases
	// with the rest of her.
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{bobsPhoto}) {
		t.Errorf("photos deleted = %+v, want bob's photo once", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Errorf("%d photo calls outside the transaction", photos.outsideTx)
	}
}
