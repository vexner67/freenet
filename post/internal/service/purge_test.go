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
	posts := service.NewPostService(store.Posts(), store.Likes(), fakeTx{}, notifier)
	comments := service.NewCommentService(store.Posts(), store.Comments(), fakeTx{}, notifier)
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), Comments: store.Comments(), Tx: fakeTx{}, Notifier: notifier,
	})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	annsPost := write(t, posts, ann, "запись Анны")[0]
	bobsPost := write(t, posts, bob, "запись Петра")[0]
	content, err := post.NewContent("Пётр на стене Анны", nil)
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
}
