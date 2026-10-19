package holdtest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// Written is what the test of the writing of deleted pages works with.
type Written struct {
	Posts interface {
		Save(ctx context.Context, p *post.Post) error
		Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
		SetArchived(ctx context.Context, id uuid.UUID, archived bool) error
		ListReposts(ctx context.Context, postID, before uuid.UUID, limit int, archived bool) ([]*post.Post, error)
		CountByWall(ctx context.Context, wallID uuid.UUID, pick service.WallPick) (int, error)
		ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error)
		CountByAuthor(ctx context.Context, authorID uuid.UUID) (int, error)
	}
	Likes interface {
		Like(ctx context.Context, postID, userID uuid.UUID) (int, error)
		LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error)
		CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
	}
	Comments interface {
		Save(ctx context.Context, c *post.Comment) error
		ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error)
	}
	CommentLikes interface {
		Like(ctx context.Context, commentID, userID uuid.UUID) (int, error)
		LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error)
		CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
	}
	Topics interface {
		Save(ctx context.Context, t *post.Topic) error
		Get(ctx context.Context, id uuid.UUID) (*post.Topic, error)
		List(ctx context.Context, clubID uuid.UUID, query string, after *service.TopicCursor, limit int) ([]*post.Topic, error)
		Count(ctx context.Context, clubID uuid.UUID, query string) (int, error)
	}
	Messages interface {
		Save(ctx context.Context, m *post.TopicMessage) error
		List(ctx context.Context, topicID, after uuid.UUID, limit int) ([]*post.TopicMessage, error)
	}
	Albums interface {
		Save(ctx context.Context, a *post.Album) error
		Get(ctx context.Context, id uuid.UUID) (*post.Album, error)
	}
	Photos interface {
		Add(ctx context.Context, albumID uuid.UUID, photos []*post.AlbumPhoto, now time.Time) error
		Get(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, error)
		List(ctx context.Context, albumID, after uuid.UUID, limit int) ([]*post.AlbumPhoto, error)
	}
	PhotoLikes interface {
		Like(ctx context.Context, photoID, userID uuid.UUID) (int, error)
		LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error)
		CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
	}
	PhotoComments interface {
		Save(ctx context.Context, c *post.PhotoComment) error
		List(ctx context.Context, photoID, after uuid.UUID, limit int) ([]*post.PhotoComment, error)
	}
	// Holders hide the pages between them.
	Holders []service.ReactionHolder
	// Pages tell which pages are deleted, each of them the same.
	Pages []service.Pages
}

// writing is what Ann, Bob and Cat wrote to one another.
type writing struct {
	s        Written
	ann, cat uuid.UUID
	club     uuid.UUID
	post     *post.Post
	topic    *post.Topic
	album    *post.Album
	photo    *post.AlbumPhoto
}

func (w *writing) say(ctx context.Context, t *testing.T, wallID, authorID uuid.UUID, c post.Content) *post.Post {
	t.Helper()

	p, err := post.New(wallID, authorID, c, time.Now())
	must(t, err)
	must(t, w.s.Posts.Save(ctx, p))

	return p
}

func write(ctx context.Context, t *testing.T, s Written, bob uuid.UUID) *writing {
	t.Helper()

	now := time.Now()
	w := &writing{s: s, ann: uuid.New(), cat: uuid.New(), club: uuid.New()}
	dan := uuid.New()

	// A post of Ann's: both comment it and share it, Dan's repost is put
	// away, Bob writes on her wall and Cat on his.
	w.post = w.say(ctx, t, w.ann, w.ann, post.RestoreContent("запись", nil, uuid.Nil))
	onAnns := w.say(ctx, t, w.ann, bob, post.RestoreContent("на стене Анны", nil, uuid.Nil))
	bobs := w.say(ctx, t, bob, bob, post.RestoreContent("запись Боба", nil, uuid.Nil))
	w.say(ctx, t, bob, w.cat, post.RestoreContent("на стене Боба", nil, uuid.Nil))

	var bobsComment *post.Comment
	for _, who := range []uuid.UUID{bob, w.cat} {
		c, err := post.NewComment(w.post.ID(), who, post.RestoreCommentContent("комментарий", nil), now)
		must(t, err)
		must(t, s.Comments.Save(ctx, c))

		if who == bob {
			bobsComment = c
		}

		w.say(ctx, t, who, who, post.RestoreContent("", nil, w.post.ID()))
	}

	away := w.say(ctx, t, dan, dan, post.RestoreContent("", nil, w.post.ID()))
	must(t, s.Posts.SetArchived(ctx, away.ID(), true))

	// Topics of a club: Cat's, where both write, and Bob's.
	var err error
	w.topic, err = post.NewTopic(w.club, w.cat, "Тема", false, now)
	must(t, err)
	must(t, s.Topics.Save(ctx, w.topic))
	bobsTopic, err := post.NewTopic(w.club, bob, "Тема Боба", false, now)
	must(t, err)
	must(t, s.Topics.Save(ctx, bobsTopic))

	for _, who := range []uuid.UUID{bob, w.cat} {
		m, err := post.NewTopicMessage(w.topic, who, post.RestoreCommentContent("сообщение", nil), false, now)
		must(t, err)
		must(t, s.Messages.Save(ctx, m))
	}

	// An album of the club with a photo of each, both comment Cat's.
	w.album, err = post.NewAlbum(w.club, "Лето", "", now)
	must(t, err)
	must(t, s.Albums.Save(ctx, w.album))

	var bobsPhoto *post.AlbumPhoto
	for _, who := range []uuid.UUID{bob, w.cat} {
		p, err := post.NewAlbumPhoto(w.album, uuid.Must(uuid.NewV7()), who, "", now)
		must(t, err)
		must(t, s.Photos.Add(ctx, w.album.ID(), []*post.AlbumPhoto{p}, now))

		if who == bob {
			bobsPhoto = p
		} else {
			w.photo = p
		}
	}

	for _, who := range []uuid.UUID{bob, w.cat} {
		pc, err := post.NewPhotoComment(w.photo.ID(), who, "красиво", now)
		must(t, err)
		must(t, s.PhotoComments.Save(ctx, pc))
	}

	// Cat likes what Ann wrote and what Bob wrote.
	for _, id := range []uuid.UUID{w.post.ID(), bobs.ID(), onAnns.ID()} {
		_, err = s.Likes.Like(ctx, id, w.cat)
		must(t, err)
	}
	_, err = s.CommentLikes.Like(ctx, bobsComment.ID(), w.cat)
	must(t, err)
	for _, id := range []uuid.UUID{w.photo.ID(), bobsPhoto.ID()} {
		_, err = s.PhotoLikes.Like(ctx, id, w.cat)
		must(t, err)
	}

	return w
}

// shown is how much of everything a reader finds: each list beside the
// number that promises it.
func (w *writing) shown(ctx context.Context, t *testing.T) [][2]int {
	t.Helper()

	const all = 100

	s := w.s
	nothing := uuid.Nil

	p, err := s.Posts.Get(ctx, w.post.ID())
	must(t, err)
	comments, err := s.Comments.ListByPost(ctx, w.post.ID(), nothing, all)
	must(t, err)
	reposts, err := s.Posts.ListReposts(ctx, w.post.ID(), nothing, all, false)
	must(t, err)

	wall, err := s.Posts.CountByWall(ctx, w.ann, service.WallPick{})
	must(t, err)
	own, err := s.Posts.ListByAuthor(ctx, w.cat, nothing, all)
	must(t, err)
	ownCount, err := s.Posts.CountByAuthor(ctx, w.cat)
	must(t, err)

	topic, err := s.Topics.Get(ctx, w.topic.ID())
	must(t, err)
	messages, err := s.Messages.List(ctx, w.topic.ID(), nothing, all)
	must(t, err)
	topics, err := s.Topics.List(ctx, w.club, "", nil, all)
	must(t, err)
	topicCount, err := s.Topics.Count(ctx, w.club, "")
	must(t, err)

	album, err := s.Albums.Get(ctx, w.album.ID())
	must(t, err)
	photos, err := s.Photos.List(ctx, w.album.ID(), nothing, all)
	must(t, err)
	photo, err := s.Photos.Get(ctx, w.photo.ID())
	must(t, err)
	photoComments, err := s.PhotoComments.List(ctx, w.photo.ID(), nothing, all)
	must(t, err)

	faves := func(likes interface {
		LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error)
		CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
	}) [2]int {
		list, err := likes.LikedBy(ctx, w.cat, nil, all)
		must(t, err)
		n, err := likes.CountLikedBy(ctx, w.cat)
		must(t, err)

		return [2]int{len(list), n}
	}

	return [][2]int{
		{len(comments), p.Counters().Comments},
		{len(reposts), p.Counters().Reposts},
		{wall, wall},
		{len(own), ownCount},
		{len(messages), topic.Messages()},
		{len(topics), topicCount},
		{len(photos), album.Photos()},
		{len(photoComments), photo.Comments()},
		faves(s.Likes),
		faves(s.CommentLikes),
		faves(s.PhotoLikes),
	}
}

func (w *writing) want(ctx context.Context, t *testing.T, when string, want ...int) {
	t.Helper()

	got := w.shown(ctx, t)
	for i, pair := range got {
		if pair[0] != pair[1] || pair[0] != want[i] {
			t.Fatalf("%s: lists and their counters = %v, want both %v", when, got, want)
		}
	}
}

// RunWritten deletes and restores the page of one who wrote among
// others: every list stays as long as its counter says.
func RunWritten(t *testing.T, s Written) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bob := uuid.New()
	w := write(ctx, t, s, bob)

	each := func(fn func(h service.ReactionHolder) error) {
		for _, h := range s.Holders {
			must(t, fn(h))
		}
	}

	// Comments, reposts (the archived one is never told), the wall of
	// Ann, what Cat wrote, messages, topics, photos, comments of a photo
	// and the posts, comments and photos Cat likes.
	everyone := []int{2, 2, 2, 2, 2, 2, 2, 2, 3, 1, 2}
	w.want(ctx, t, "everyone is here", everyone...)

	// Twice, as the outbox may deliver it.
	// What is read by its ID asks whose page is deleted.
	deleted := func(when string, want bool) {
		t.Helper()

		for _, p := range s.Pages {
			gone, err := p.Deleted(ctx, []uuid.UUID{w.ann, bob, uuid.New()})
			if err != nil || gone[bob] != want || gone[w.ann] || len(gone) > 1 {
				t.Fatalf("%s: Deleted = %v, %v; want bob: %v", when, gone, err, want)
			}
		}
	}
	deleted("everyone is here", false)

	each(func(h service.ReactionHolder) error { return h.Hold(ctx, bob) })
	each(func(h service.ReactionHolder) error { return h.Hold(ctx, bob) })
	w.want(ctx, t, "bob deleted the page", 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1)
	deleted("bob deleted the page", true)

	each(func(h service.ReactionHolder) error { return h.Release(ctx, bob) })
	w.want(ctx, t, "bob is back", everyone...)
	deleted("bob is back", false)

	// Erased for good, the page is no longer hidden: what it wrote goes
	// by its own ways.
	each(func(h service.ReactionHolder) error { return h.Hold(ctx, bob) })
	each(func(h service.ReactionHolder) error { return h.Forget(ctx, bob) })
	w.want(ctx, t, "bob is erased", everyone...)
	deleted("bob is erased", false)
}
