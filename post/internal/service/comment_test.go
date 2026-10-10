package service_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
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
		posts:    service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}, Switches: store.Posts()}),
		comments: service.NewCommentService(service.CommentDeps{Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: notifier, Photos: &fakePhotos{}, Moderators: fakeModerators{}}),
		notifier: notifier,
	}
}

func (f commentFixture) comment(t *testing.T, author, postID uuid.UUID, texts ...string) []*post.Comment {
	t.Helper()

	comments := make([]*post.Comment, 0, len(texts))
	for _, text := range texts {
		c, err := f.comments.Create(context.Background(), service.CommentInput{AuthorID: author, PostID: postID, Content: post.RestoreCommentContent(post.CommentText(text), nil)})
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
		page, err := f.comments.List(ctx, uuid.Nil, p.ID(), cursor, 2)
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

	empty, err := f.comments.List(ctx, uuid.Nil, write(t, f.posts, bob, "пусто")[0].ID(), uuid.Nil, 0)
	if err != nil || len(empty.Comments) != 0 || empty.Total != 0 || empty.NextCursor != uuid.Nil {
		t.Fatalf("no comments: %+v, %v", empty, err)
	}

	if _, err = f.comments.List(ctx, uuid.Nil, uuid.New(), uuid.Nil, 20); !errors.Is(err, post.ErrNotFound) {
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

	if _, err := f.comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: uuid.New(), Content: post.RestoreCommentContent("куда?", nil)}); !errors.Is(err, post.ErrNotFound) {
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

	page, err := f.comments.List(ctx, uuid.Nil, p.ID(), uuid.Nil, 20)
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

	// Deleted by the author of the post, the comment takes every
	// notification of it all the same: of it, its replies, its likes.
	if err := f.comments.Delete(ctx, ann, theirs.ID()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.notifier.forgottenComments, []uuid.UUID{theirs.ID()}) {
		t.Fatalf("forgotten comments = %v, want bob's", f.notifier.forgottenComments)
	}
}

func TestCommentService_Reply(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]
	other := write(t, f.posts, ann, "другая")[0]
	content := post.RestoreCommentContent("Пётр, согласна", nil)

	bobs := f.comment(t, bob, p.ID(), "и тебе")[0]
	f.notifier.notified = nil

	// Cat answers bob: bob hears of the answer, ann of a new comment.
	reply, err := f.comments.Create(ctx, service.CommentInput{AuthorID: cat, PostID: p.ID(), ParentID: bobs.ID(), Content: content})
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if reply.ReplyTo() != bobs.ID() || reply.PostID() != p.ID() {
		t.Fatalf("reply answers %v under %v", reply.ReplyTo(), reply.PostID())
	}
	want := []service.Event{
		{Kind: service.EventCommentReply, RecipientID: bob, ActorID: cat, PostID: p.ID(), CommentID: reply.ID()},
		{Kind: service.EventComment, RecipientID: ann, ActorID: cat, PostID: p.ID(), CommentID: reply.ID()},
	}
	if !slices.Equal(f.notifier.notified, want) {
		t.Fatalf("notified = %+v, want %+v", f.notifier.notified, want)
	}

	// Ann answers cat on her own post: cat only, once.
	f.notifier.notified = nil
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: p.ID(), ParentID: reply.ID(), Content: content}); err != nil {
		t.Fatal(err)
	}
	if len(f.notifier.notified) != 1 || f.notifier.notified[0].Kind != service.EventCommentReply || f.notifier.notified[0].RecipientID != cat {
		t.Fatalf("notified = %+v, want cat's reply", f.notifier.notified)
	}

	// Bob answers cat and names her, ann, dan and himself: cat hears of
	// the answer and ann of the comment, as before, dan alone of his name.
	dan := uuid.New()
	f.notifier.notified = nil
	named, err := f.comments.Create(ctx, service.CommentInput{
		AuthorID: bob, PostID: p.ID(), ParentID: reply.ID(), Content: content, Mentions: post.Mentions{cat, ann, dan, bob},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = []service.Event{
		{Kind: service.EventCommentReply, RecipientID: cat, ActorID: bob, PostID: p.ID(), CommentID: named.ID()},
		{Kind: service.EventComment, RecipientID: ann, ActorID: bob, PostID: p.ID(), CommentID: named.ID()},
		{Kind: service.EventCommentMention, RecipientID: dan, ActorID: bob, PostID: p.ID(), CommentID: named.ID()},
	}
	if !slices.Equal(f.notifier.notified, want) {
		t.Fatalf("notified = %+v, want %+v", f.notifier.notified, want)
	}

	// Cat answers her own comment: only ann hears, of a new comment.
	f.notifier.notified = nil
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: cat, PostID: p.ID(), ParentID: reply.ID(), Content: content}); err != nil {
		t.Fatal(err)
	}
	if len(f.notifier.notified) != 1 || f.notifier.notified[0].Kind != service.EventComment {
		t.Fatalf("answering yourself: notified = %+v, want ann's comment only", f.notifier.notified)
	}

	// A reply under another post than the comment's is refused.
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: cat, PostID: other.ID(), ParentID: bobs.ID(), Content: content}); !errors.Is(err, post.ErrReplyOtherPost) {
		t.Fatalf("reply under another post: %v", err)
	}
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: cat, PostID: p.ID(), ParentID: uuid.New(), Content: content}); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("reply to nothing: %v", err)
	}

	// The comment answered is deleted: the reply stays, answering nothing.
	if err = f.comments.Delete(ctx, bob, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	page, err := f.comments.List(ctx, uuid.Nil, p.ID(), uuid.Nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Comments[0].ID() != reply.ID() || page.Comments[0].ReplyTo() != uuid.Nil {
		t.Fatalf("after deleting the comment answered: %v answers %v", page.Comments[0].ID(), page.Comments[0].ReplyTo())
	}
}

func TestCommentService_Like(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	p := write(t, f.posts, ann, "привет")[0]
	c := f.comment(t, bob, p.ID(), "и тебе")[0]
	f.notifier.notified = nil

	for range 2 { // idempotent
		if likes, err := f.comments.Like(ctx, ann, c.ID()); err != nil || likes != 1 {
			t.Fatalf("Like = %d, %v", likes, err)
		}
	}
	if likes, err := f.comments.Like(ctx, bob, c.ID()); err != nil || likes != 2 {
		t.Fatalf("bob likes his own = %d, %v", likes, err)
	}

	like := service.Event{Kind: service.EventCommentLike, RecipientID: bob, ActorID: ann, PostID: p.ID(), CommentID: c.ID()}
	// Twice: notification dedups the repeat; bob's own like tells nobody.
	if !slices.Equal(f.notifier.notified, []service.Event{like, like}) {
		t.Fatalf("notified = %+v", f.notifier.notified)
	}

	page, err := f.comments.List(ctx, ann, p.ID(), uuid.Nil, 20)
	if err != nil || !page.Liked[c.ID()] || page.Comments[0].Likes() != 2 {
		t.Fatalf("ann's view: liked %v, likes %d, %v", page.Liked, page.Comments[0].Likes(), err)
	}
	if anon, _ := f.comments.List(ctx, uuid.Nil, p.ID(), uuid.Nil, 20); len(anon.Liked) != 0 {
		t.Fatalf("an anonymous viewer likes %v", anon.Liked)
	}

	if likes, err := f.comments.Unlike(ctx, ann, c.ID()); err != nil || likes != 1 {
		t.Fatalf("Unlike = %d, %v", likes, err)
	}
	if !slices.Equal(f.notifier.retracted, []service.Event{like}) {
		t.Fatalf("retracted = %+v", f.notifier.retracted)
	}

	if _, err := f.comments.Like(ctx, ann, uuid.New()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("like a missing comment: %v", err)
	}
	if f.notifier.outsideTx != 0 {
		t.Fatalf("%d notifications outside the transaction", f.notifier.outsideTx)
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

func TestCommentService_Last(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	busy := write(t, f.posts, ann, "привет")[0]
	quiet := write(t, f.posts, ann, "тихо")[0]
	silent := write(t, f.posts, ann, "молчание")[0]
	hidden := write(t, f.posts, ann, "в архив")[0]

	talk := f.comment(t, bob, busy.ID(), "1", "2", "3", "4", "5")
	f.comment(t, bob, quiet.ID(), "одно")
	f.comment(t, bob, hidden.ID(), "не видно")
	if err := f.posts.Archive(ctx, ann, hidden.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.comments.Like(ctx, ann, talk[4].ID()); err != nil {
		t.Fatal(err)
	}

	page, err := f.comments.Last(ctx, ann, []uuid.UUID{quiet.ID(), busy.ID(), silent.ID(), hidden.ID(), uuid.New()})
	if err != nil {
		t.Fatalf("Last: %v", err)
	}

	// The newest three of each post, oldest first; none of an archived one.
	got := map[uuid.UUID][]string{}
	for _, c := range page.Comments {
		got[c.PostID()] = append(got[c.PostID()], c.Text().String())
	}
	want := map[uuid.UUID][]string{busy.ID(): {"3", "4", "5"}, quiet.ID(): {"одно"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Last = %v, want %v", got, want)
	}
	if len(page.Liked) != 1 || !page.Liked[talk[4].ID()] {
		t.Fatalf("liked = %v", page.Liked)
	}

	// An anonymous reader likes nothing; no posts, no comments.
	if page, err = f.comments.Last(ctx, uuid.Nil, []uuid.UUID{busy.ID()}); err != nil || len(page.Comments) != service.LastComments || page.Liked != nil {
		t.Fatalf("anonymous = %+v, %v", page, err)
	}
	if page, err = f.comments.Last(ctx, ann, nil); err != nil || len(page.Comments) != 0 {
		t.Fatalf("of no posts = %+v, %v", page, err)
	}
	if _, err = f.comments.Last(ctx, ann, make([]uuid.UUID, service.MaxPageSize+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many ids: %v", err)
	}
}

func TestCommentService_Photos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: photos,
		Moderators: fakeModerators{},
	})
	comments := service.NewCommentService(service.CommentDeps{
		Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: photos,
		Moderators: fakeModerators{},
	})
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	bobsPhoto, catsPhoto := uuid.New(), uuid.New()

	p := write(t, posts, ann, "запись")[0]

	withPhoto := func(author, photo uuid.UUID) *post.Comment {
		t.Helper()

		content, err := post.NewCommentContent("", post.Photos{photo})
		if err != nil {
			t.Fatal(err)
		}
		c, err := comments.Create(ctx, service.CommentInput{AuthorID: author, PostID: p.ID(), Content: content})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return c
	}

	bobs := withPhoto(bob, bobsPhoto)
	withPhoto(cat, catsPhoto)
	if len(photos.published) != 2 || photos.published[0].owner != bob || !slices.Equal(photos.published[0].photos, []uuid.UUID{bobsPhoto}) {
		t.Fatalf("published = %+v, want each commenter's photo", photos.published)
	}

	// Ann, the author of the post, deletes bob's comment: bob's photo goes.
	if err := comments.Delete(ctx, ann, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{bobsPhoto}) {
		t.Fatalf("deleted = %+v, want bob's photo", photos.deleted)
	}

	// The post takes cat's comment with its photo.
	if err := posts.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 2 || photos.deleted[1].owner != cat || !slices.Equal(photos.deleted[1].photos, []uuid.UUID{catsPhoto}) {
		t.Fatalf("deleted = %+v, want cat's photo with the post", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d photo calls outside the transaction", photos.outsideTx)
	}
}

func TestCommentService_ForWall(t *testing.T) {
	t.Parallel()

	f := newCommentFixture()
	ctx := context.Background()
	club, admin, ann := uuid.New(), uuid.New(), uuid.New()
	content := post.RestoreCommentContent("от группы", nil)

	// Ann writes on the club's wall; the admin answers for the club.
	annsContent, _ := post.NewContent("вопрос", nil)
	annsPost, err := f.posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: ann, Content: annsContent, ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}
	f.notifier.notified = nil

	forClub, err := f.comments.Create(ctx, service.CommentInput{AuthorID: admin, PostID: annsPost.ID(), Content: content, FromWall: true})
	if err != nil || !forClub.FromWall() || forClub.AuthorID() != admin {
		t.Fatalf("Create for the club = %+v, %v", forClub, err)
	}
	if n := f.notifier.notified; len(n) != 1 || n[0].Kind != service.EventComment || n[0].RecipientID != ann || n[0].ActorID != club {
		t.Fatalf("ann hears from the club, not the admin: %+v", n)
	}

	// Answers to it and its likes tell nobody: its author is not shown.
	f.notifier.notified = nil
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: annsPost.ID(), ParentID: forClub.ID(), Content: content}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.comments.Like(ctx, ann, forClub.ID()); err != nil {
		t.Fatal(err)
	}
	if n := f.notifier.notified; len(n) != 0 {
		t.Fatalf("the admin hears of answers and likes: %+v", n)
	}

	// Nobody speaks for their own wall but themselves.
	own := write(t, f.posts, ann, "моя запись")[0]
	if _, err = f.comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: own.ID(), Content: content, FromWall: true}); !errors.Is(err, post.ErrFromOwnWall) {
		t.Fatalf("for one's own wall: %v", err)
	}
}
