package service_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

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
	posts := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{}})
	comments := service.NewCommentService(service.CommentDeps{Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{}})
	suggestions := inmem.NewSuggestionRepository()
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Comments: store.Comments(), Suggestions: suggestions,
		Tx: fakeTx{}, Notifier: notifier, Photos: photos,
	})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	annsPhoto, bobsPhoto := uuid.New(), uuid.New()
	annsContent, err := post.NewContent("запись Анны", post.Photos{annsPhoto})
	if err != nil {
		t.Fatal(err)
	}
	annsPost, err := posts.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: annsContent})
	if err != nil {
		t.Fatal(err)
	}
	bobsPost := write(t, posts, bob, "запись Петра")[0]
	content, err := post.NewContent("Пётр на стене Анны", post.Photos{bobsPhoto})
	if err != nil {
		t.Fatal(err)
	}
	onAnnsWall, err := posts.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: content})
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
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: bobsPost.ID(), Content: post.RestoreCommentContent(text, nil)}); err != nil {
		t.Fatal(err)
	}
	// Bob comments on his post, and ann likes it.
	bobsComment, err := comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: bobsPost.ID(), Content: post.RestoreCommentContent("моя запись", nil)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = comments.Like(ctx, ann, bobsComment.ID()); err != nil {
		t.Fatal(err)
	}
	// Cat comments on ann's post with a photo, which goes with the post.
	cat, catsPhoto := uuid.New(), uuid.New()
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: cat, PostID: annsPost.ID(), Content: post.RestoreCommentContent("", post.Photos{catsPhoto})}); err != nil {
		t.Fatal(err)
	}

	club := uuid.New()
	suggestion, err := post.NewSuggestion(club, ann, annsContent, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = suggestions.Save(ctx, suggestion); err != nil {
		t.Fatal(err)
	}

	notifier.forgotten = nil
	if err = purge.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if n, _ := suggestions.Count(ctx, club, uuid.Nil); n != 0 {
		t.Fatalf("ann's suggestions left: %d", n)
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
	if c := kept.Counters(); c.Likes != 1 || c.Comments != 1 || c.Reposts != 0 {
		t.Errorf("bob's post counters = %+v, want bob's like and comment only", c)
	}
	if page, err := comments.List(ctx, uuid.Nil, bobsPost.ID(), uuid.Nil, 20); err != nil || page.Comments[0].Likes() != 0 {
		t.Errorf("bob's comment keeps ann's like: %v", err)
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

	// Cat's photo goes with the comment on ann's post, bob's with his post
	// on ann's wall; ann's own media erases with the rest of her.
	wantPhotos := []photoCall{{cat, []uuid.UUID{catsPhoto}}, {bob, []uuid.UUID{bobsPhoto}}}
	if !reflect.DeepEqual(photos.deleted, wantPhotos) {
		t.Errorf("photos deleted = %+v, want %+v", photos.deleted, wantPhotos)
	}
	if photos.outsideTx != 0 {
		t.Errorf("%d photo calls outside the transaction", photos.outsideTx)
	}
}
