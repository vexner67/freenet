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

type commentFixture struct {
	posts    *service.PostService
	comments *service.CommentService
	notifier *fakeNotifier
}

func newCommentFixture() commentFixture {
	store := inmem.NewStore()
	notifier := &fakeNotifier{}

	return commentFixture{
		posts:    service.NewPostService(service.PostDeps{Posts: store.Posts(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}}),
		comments: service.NewCommentService(store.Posts(), store.Comments(), fakeTx{}, notifier),
		notifier: notifier,
	}
}

func (f commentFixture) comment(t *testing.T, author, postID uuid.UUID, texts ...string) []*post.Comment {
	t.Helper()

	comments := make([]*post.Comment, 0, len(texts))
	for _, text := range texts {
		c, err := f.comments.Create(context.Background(), author, postID, post.CommentText(text))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		comments = append(comments, c)
	}

	return comments
}

func commentTexts(comments []*post.Comment) []string {
	out := make([]string, len(comments))
	for i, c := range comments {
		out[i] = c.Text().String()
	}
	return out
}

func TestCommentService_List(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]
	other := write(t, f.posts, ann, "другая")[0]

	f.comment(t, bob, p.ID(), "1", "2", "3")
	f.comment(t, ann, p.ID(), "4", "5")
	f.comment(t, bob, other.ID(), "не сюда")

	var (
		got    [][]string
		cursor uuid.UUID
	)

	for {
		page, err := f.comments.List(ctx, p.ID(), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Fatalf("total = %d, want 5", page.Total)
		}

		got = append(got, commentTexts(page.Comments))

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	// Oldest first, as a conversation reads.
	want := [][]string{{"1", "2"}, {"3", "4"}, {"5"}}
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || got[i][0] != want[i][0] || got[i][len(got[i])-1] != want[i][len(want[i])-1] {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}

	counted, _, err := f.posts.Get(ctx, uuid.Nil, p.ID())
	if err != nil || counted.Counters().Comments != 5 {
		t.Fatalf("post counters = %v, %v", counted.Counters(), err)
	}

	empty, err := f.comments.List(ctx, write(t, f.posts, bob, "пусто")[0].ID(), uuid.Nil, 0)
	if err != nil || len(empty.Comments) != 0 || empty.Total != 0 || empty.NextCursor != uuid.Nil {
		t.Fatalf("no comments: %+v, %v", empty, err)
	}

	if _, err = f.comments.List(ctx, uuid.New(), uuid.Nil, 20); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("missing post: %v", err)
	}
}

func TestCommentService_Create(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]

	c := f.comment(t, bob, p.ID(), "и тебе")[0]
	if c.PostID() != p.ID() || c.AuthorID() != bob || c.Text() != "и тебе" {
		t.Fatalf("comment = %v %v %q", c.PostID(), c.AuthorID(), c.Text())
	}

	if _, err := f.comments.Create(ctx, bob, uuid.New(), "куда?"); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("comment a missing post: %v", err)
	}
}

func TestCommentService_Delete(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]
	comments := f.comment(t, bob, p.ID(), "от боба", "ещё от боба")

	if err := f.comments.Delete(ctx, cat, comments[0].ID()); !errors.Is(err, post.ErrCannotDeleteComment) {
		t.Fatalf("stranger: got %v, want ErrCannotDeleteComment", err)
	}

	if err := f.comments.Delete(ctx, bob, comments[0].ID()); err != nil {
		t.Fatalf("author of the comment: %v", err)
	}

	if err := f.comments.Delete(ctx, ann, comments[1].ID()); err != nil {
		t.Fatalf("author of the post: %v", err)
	}

	if err := f.comments.Delete(ctx, bob, comments[0].ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("twice: got %v, want ErrCommentNotFound", err)
	}

	page, err := f.comments.List(ctx, p.ID(), uuid.Nil, 20)
	if err != nil || len(page.Comments) != 0 || page.Total != 0 {
		t.Fatalf("after deleting: %+v, %v", page, err)
	}
}

// Deleting a post takes its likes and comments with it, as the foreign keys
// do in PostgreSQL.
func TestDeletePostRemovesLikesAndComments(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]
	c := f.comment(t, bob, p.ID(), "и тебе")[0]

	if _, err := f.posts.Like(ctx, bob, p.ID()); err != nil {
		t.Fatal(err)
	}

	if err := f.posts.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}

	if err := f.comments.Delete(ctx, bob, c.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("comment of a deleted post: %v", err)
	}
	if _, err := f.posts.Like(ctx, bob, p.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("like a deleted post: %v", err)
	}
}

func TestCommentService_Notifies(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]

	theirs := f.comment(t, bob, p.ID(), "и тебе")[0]
	f.comment(t, ann, p.ID(), "спасибо") // nobody is notified about their own post

	want := service.Event{Kind: service.EventComment, RecipientID: ann, ActorID: bob, PostID: p.ID(), CommentID: theirs.ID()}
	if len(f.notifier.notified) != 1 || f.notifier.notified[0] != want {
		t.Fatalf("notified = %+v, want %+v", f.notifier.notified, want)
	}

	// Deleted by the author of the post, the comment is taken back all the same.
	if err := f.comments.Delete(ctx, ann, theirs.ID()); err != nil {
		t.Fatal(err)
	}
	if len(f.notifier.retracted) != 1 || f.notifier.retracted[0] != want {
		t.Fatalf("retracted = %+v, want %+v", f.notifier.retracted, want)
	}
}

func TestCommentService_GetMany(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	p := write(t, f.posts, uuid.New(), "привет")[0]
	comments := f.comment(t, uuid.New(), p.ID(), "раз", "два")

	got, err := f.comments.GetMany(ctx, []uuid.UUID{comments[1].ID(), uuid.New()})
	if err != nil || len(got) != 1 || got[0].ID() != comments[1].ID() {
		t.Fatalf("GetMany = %v, %v", got, err)
	}

	if _, err = f.comments.GetMany(ctx, make([]uuid.UUID, service.MaxPageSize+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many ids: %v", err)
	}
}
