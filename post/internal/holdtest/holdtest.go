// Package holdtest is the test every storage of held reactions passes:
// PostgreSQL and the one in memory behave alike.
package holdtest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// Storage is what the test works with.
type Storage struct {
	Posts interface {
		Save(ctx context.Context, p *post.Post) error
		Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
		Delete(ctx context.Context, id uuid.UUID) error
	}
	Likes interface {
		Like(ctx context.Context, postID, userID uuid.UUID) (int, error)
		Likers(ctx context.Context, postID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error)
		CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
	}
	Comments interface {
		Save(ctx context.Context, c *post.Comment) error
		Get(ctx context.Context, id uuid.UUID) (*post.Comment, error)
	}
	CommentLikes interface {
		Like(ctx context.Context, commentID, userID uuid.UUID) (int, error)
	}
	Polls interface {
		Save(ctx context.Context, p *post.Poll) error
		Get(ctx context.Context, postID uuid.UUID) (*post.Poll, error)
		Vote(ctx context.Context, postID, userID uuid.UUID, choice post.PollChoice, at time.Time) error
	}
	Topics interface {
		Save(ctx context.Context, t *post.Topic) error
	}
	Messages interface {
		Save(ctx context.Context, m *post.TopicMessage) error
		Get(ctx context.Context, id uuid.UUID) (*post.TopicMessage, error)
		Like(ctx context.Context, messageID, userID uuid.UUID) (int, error)
	}
	Albums interface {
		Save(ctx context.Context, a *post.Album) error
	}
	Photos interface {
		Add(ctx context.Context, albumID uuid.UUID, photos []*post.AlbumPhoto, now time.Time) error
		Get(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, error)
	}
	PhotoLikes interface {
		Like(ctx context.Context, photoID, userID uuid.UUID) (int, error)
	}
	PhotoComments interface {
		Save(ctx context.Context, c *post.PhotoComment) error
		Get(ctx context.Context, id uuid.UUID) (*post.PhotoComment, error)
		Like(ctx context.Context, commentID, userID uuid.UUID) (int, error)
	}
	// Holders keep aside all of the above between them.
	Holders []service.ReactionHolder
}

// fixture is what Ann wrote and Bob and Cat liked.
type fixture struct {
	s       Storage
	post    *post.Post
	gone    *post.Post
	comment *post.Comment
	message *post.TopicMessage
	photo   *post.AlbumPhoto
	reply   *post.PhotoComment
}

func must(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func build(ctx context.Context, t *testing.T, s Storage, ann uuid.UUID) *fixture {
	t.Helper()

	now := time.Now()
	f := &fixture{s: s}

	var err error

	f.post, err = post.New(ann, ann, post.NewPolled("опрос", nil), now)
	must(t, err)
	must(t, s.Posts.Save(ctx, f.post))

	f.gone, err = post.New(ann, ann, post.RestoreContent("ненадолго", nil, uuid.Nil), now)
	must(t, err)
	must(t, s.Posts.Save(ctx, f.gone))

	title, err := post.NewPollTitle("Кошки или собаки?")
	must(t, err)
	options, err := post.NewPollOptions([]string{"кошки", "собаки"})
	must(t, err)
	poll, err := post.NewPoll(f.post.ID(), post.PollDraft{Title: title, Options: options, Multiple: true}, now)
	must(t, err)
	must(t, s.Polls.Save(ctx, poll))

	f.comment, err = post.NewComment(f.post.ID(), ann, post.RestoreCommentContent("комментарий", nil), now)
	must(t, err)
	must(t, s.Comments.Save(ctx, f.comment))

	topic, err := post.NewTopic(uuid.New(), ann, "Тема", false, now)
	must(t, err)
	must(t, s.Topics.Save(ctx, topic))
	f.message, err = post.NewTopicMessage(topic, ann, post.RestoreCommentContent("сообщение", nil), false, now)
	must(t, err)
	must(t, s.Messages.Save(ctx, f.message))

	album, err := post.NewAlbum(ann, "Лето", "", now)
	must(t, err)
	must(t, s.Albums.Save(ctx, album))
	f.photo, err = post.NewAlbumPhoto(album, uuid.Must(uuid.NewV7()), ann, "", now)
	must(t, err)
	must(t, s.Photos.Add(ctx, album.ID(), []*post.AlbumPhoto{f.photo}, now))
	f.reply, err = post.NewPhotoComment(f.photo.ID(), ann, "красиво", now)
	must(t, err)
	must(t, s.PhotoComments.Save(ctx, f.reply))

	return f
}

// react is everything one user can like and vote for.
func (f *fixture) react(ctx context.Context, t *testing.T, userID uuid.UUID, choice post.PollChoice) {
	t.Helper()

	likes := []func() (int, error){
		func() (int, error) { return f.s.Likes.Like(ctx, f.post.ID(), userID) },
		func() (int, error) { return f.s.Likes.Like(ctx, f.gone.ID(), userID) },
		func() (int, error) { return f.s.CommentLikes.Like(ctx, f.comment.ID(), userID) },
		func() (int, error) { return f.s.Messages.Like(ctx, f.message.ID(), userID) },
		func() (int, error) { return f.s.PhotoLikes.Like(ctx, f.photo.ID(), userID) },
		func() (int, error) { return f.s.PhotoComments.Like(ctx, f.reply.ID(), userID) },
	}
	for _, like := range likes {
		_, err := like()
		must(t, err)
	}

	must(t, f.s.Polls.Vote(ctx, f.post.ID(), userID, choice, time.Now()))
}

// counts are the likes of the post, its comment, the message, the photo
// and its comment, then the voters of the poll and the votes of its
// options.
func (f *fixture) counts(ctx context.Context, t *testing.T) []int {
	t.Helper()

	p, err := f.s.Posts.Get(ctx, f.post.ID())
	must(t, err)
	c, err := f.s.Comments.Get(ctx, f.comment.ID())
	must(t, err)
	m, err := f.s.Messages.Get(ctx, f.message.ID())
	must(t, err)
	photo, err := f.s.Photos.Get(ctx, f.photo.ID())
	must(t, err)
	reply, err := f.s.PhotoComments.Get(ctx, f.reply.ID())
	must(t, err)
	poll, err := f.s.Polls.Get(ctx, f.post.ID())
	must(t, err)

	tally := poll.Tally()

	return append([]int{p.Counters().Likes, c.Likes(), m.Likes(), photo.Likes(), reply.Likes(), tally.Voters}, tally.Votes...)
}

func (f *fixture) each(ctx context.Context, t *testing.T, fn func(h service.ReactionHolder) error) {
	t.Helper()

	for _, h := range f.s.Holders {
		must(t, fn(h))
	}
}

func (f *fixture) want(ctx context.Context, t *testing.T, when string, want ...int) {
	t.Helper()

	if got := f.counts(ctx, t); !slices.Equal(got, want) {
		t.Fatalf("%s: counts = %v, want %v", when, got, want)
	}
}

// Run holds, releases and forgets the reactions of users against s.
func Run(t *testing.T, s Storage) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	f := build(ctx, t, s, ann)

	f.react(ctx, t, bob, post.PollChoice{0, 1})
	f.react(ctx, t, cat, post.PollChoice{0})
	f.want(ctx, t, "both reacted", 2, 2, 2, 2, 2, 2, 2, 1)

	hold := func(id uuid.UUID) { f.each(ctx, t, func(h service.ReactionHolder) error { return h.Hold(ctx, id) }) }
	release := func(id uuid.UUID) { f.each(ctx, t, func(h service.ReactionHolder) error { return h.Release(ctx, id) }) }
	forget := func(id uuid.UUID) { f.each(ctx, t, func(h service.ReactionHolder) error { return h.Forget(ctx, id) }) }

	// Bob's page is deleted: what he liked is not counted nor listed.
	// Twice, as the outbox may deliver it.
	hold(bob)
	hold(bob)
	f.want(ctx, t, "bob is held", 1, 1, 1, 1, 1, 1, 1, 0)

	// Nor are they among the faves of the page.
	if n, err := s.Likes.CountLikedBy(ctx, bob); err != nil || n != 0 {
		t.Fatalf("faves of bob while he is held = %d, %v", n, err)
	}

	likers, err := s.Likes.Likers(ctx, f.post.ID(), nil, 10)
	if err != nil || len(likers) != 1 || likers[0].UserID != cat {
		t.Fatalf("likers while bob is held = %+v, %v", likers, err)
	}

	// A post deleted meanwhile takes his like with it.
	must(t, s.Posts.Delete(ctx, f.gone.ID()))

	// Restored, he is counted again, where he liked when he did.
	release(bob)
	release(bob)
	f.want(ctx, t, "bob is released", 2, 2, 2, 2, 2, 2, 2, 1)

	if n, err := s.Likes.CountLikedBy(ctx, bob); err != nil || n != 1 {
		t.Fatalf("faves of bob once he is back = %d, %v", n, err)
	}

	if likers, err = s.Likes.Likers(ctx, f.post.ID(), nil, 10); err != nil || len(likers) != 2 || likers[1].UserID != bob {
		t.Fatalf("likers once bob is back = %+v, %v; want bob the older", likers, err)
	}

	// Releasing someone who is not held changes nothing.
	release(ann)
	f.want(ctx, t, "nobody to release", 2, 2, 2, 2, 2, 2, 2, 1)

	// A page erased while held leaves nothing to put back.
	hold(cat)
	forget(cat)
	release(cat)
	f.want(ctx, t, "cat is forgotten", 1, 1, 1, 1, 1, 1, 1, 1)
}
