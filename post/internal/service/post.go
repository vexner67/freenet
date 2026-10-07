package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxFeedAuthors bounds one news feed: the people a user follows plus
	// the user.
	MaxFeedAuthors = 1001
)

var (
	ErrTooManyAuthors = errors.New("too many authors")
	ErrTooManyIDs     = errors.New("too many ids")
)

type PostRepository interface {
	// Save counts a repost in the post it shares and returns
	// post.ErrNotFound when that post is gone; Delete uncounts it.
	Save(ctx context.Context, p *post.Post) error
	// Get and Delete return post.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
	// GetMany returns the posts that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ListByWall returns up to limit posts on a wall, whoever wrote them,
	// newest first, with IDs below before (uuid.Nil: from the newest).
	ListByWall(ctx context.Context, wallID, before uuid.UUID, limit int) ([]*post.Post, error)
	CountByWall(ctx context.Context, wallID uuid.UUID) (int, error)
	// ListByAuthors is ListByWall for the posts written by several authors,
	// on whichever walls.
	ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error)
}

type LikeRepository interface {
	// Like and Unlike are idempotent and return how many likes the post
	// has now; post.ErrNotFound for a missing post.
	Like(ctx context.Context, postID, userID uuid.UUID) (int, error)
	Unlike(ctx context.Context, postID, userID uuid.UUID) (int, error)
	// Liked returns which of postIDs the user likes.
	Liked(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// Page is one screen of a wall. NextCursor is uuid.Nil on the last page.
// Liked marks the posts the viewer likes; it is empty for anonymous viewers.
type Page struct {
	Posts      []*post.Post
	Liked      map[uuid.UUID]bool
	NextCursor uuid.UUID
	Total      int
}

type PostService struct {
	postRepo PostRepository
	likeRepo LikeRepository
	tx       TxManager
	notifier Notifier
	photos   PhotoKeeper
	now      func() time.Time
}

// PostDeps are what PostService works with.
type PostDeps struct {
	Posts    PostRepository
	Likes    LikeRepository
	Tx       TxManager
	Notifier Notifier
	Photos   PhotoKeeper
}

func NewPostService(d PostDeps) *PostService {
	return &PostService{
		postRepo: d.Posts,
		likeRepo: d.Likes,
		tx:       d.Tx,
		notifier: d.Notifier,
		photos:   d.Photos,
		now:      time.Now,
	}
}

// Create writes a post on the wall of wallID; the owner of a wall hears
// about posts by others, and media keeps the photos. The caller has
// checked that the owner's privacy lets the author write there and that
// the photos are the author's: post does not know users.
func (s *PostService) Create(ctx context.Context, wallID, authorID uuid.UUID, content post.Content) (*post.Post, error) {
	p, err := post.New(wallID, authorID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("post.New", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.postRepo.Save(ctx, p); err != nil {
			return errs.Wrap("s.postRepo.Save", err)
		}

		if photos := content.Photos(); len(photos) > 0 {
			if err := s.photos.Publish(ctx, authorID, photos); err != nil {
				return errs.Wrap("s.photos.Publish", err)
			}
		}

		if p.OnOwnWall() {
			return nil
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, Event{Kind: EventWallPost, RecipientID: wallID, ActorID: authorID, PostID: p.ID()}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return p, nil
}

// Repost shares postID on the author's own wall, with text that may be
// empty, as openvk's "Поделиться" does. A repost that passes another post
// on without a word is not shared itself: its original is (see
// post.Shared). The author of the shared post hears about it.
func (s *PostService) Repost(ctx context.Context, authorID, postID uuid.UUID, text post.Text) (*post.Post, error) {
	shared, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return nil, errs.Wrap("s.postRepo.Get", err)
	}

	if id := shared.Shared(); id != shared.ID() {
		if shared, err = s.postRepo.Get(ctx, id); err != nil {
			return nil, errs.Wrap("s.postRepo.Get", err)
		}
	}

	content, err := post.NewRepost(shared.ID(), text)
	if err != nil {
		return nil, errs.Wrap("post.NewRepost", err)
	}

	repost, err := post.New(authorID, authorID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("post.New", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Save still reports a shared post deleted in between.
		if err := s.postRepo.Save(ctx, repost); err != nil {
			return errs.Wrap("s.postRepo.Save", err)
		}

		if shared.AuthorID() == authorID {
			return nil
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, Event{Kind: EventRepost, RecipientID: shared.AuthorID(), ActorID: authorID, PostID: repost.ID()}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return repost, nil
}

// Delete removes a post with its photos, whoever deletes it, the author or
// the owner of the wall; a deleted repost takes its notification with it,
// since the notification is about the repost (ForgetPost).
func (s *PostService) Delete(ctx context.Context, requesterID, postID uuid.UUID) error {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	if !p.CanBeDeletedBy(requesterID) {
		return post.ErrCannotDelete
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.postRepo.Delete(ctx, postID); err != nil {
			return errs.Wrap("s.postRepo.Delete", err)
		}

		if err := s.notifier.ForgetPost(ctx, postID); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		if photos := p.Content().Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, p.AuthorID(), photos))
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// Get returns a post and whether viewerID likes it; viewerID is uuid.Nil
// for an anonymous viewer.
func (s *PostService) Get(ctx context.Context, viewerID, postID uuid.UUID) (*post.Post, bool, error) {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return nil, false, errs.Wrap("s.postRepo.Get", err)
	}

	liked, err := s.liked(ctx, viewerID, []*post.Post{p})
	if err != nil {
		return nil, false, errs.Wrap("s.liked", err)
	}

	return p, liked[p.ID()], nil
}

// GetMany returns the posts with these IDs that exist, at most
// MaxPageSize, in no particular order.
func (s *PostService) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error) {
	if len(ids) > MaxPageSize {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	posts, err := s.postRepo.GetMany(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.postRepo.GetMany", err)
	}

	return posts, nil
}

// Like is idempotent: liking twice counts once, and the notification
// service notifies the author once. It returns how many likes the post
// has now.
func (s *PostService) Like(ctx context.Context, userID, postID uuid.UUID) (int, error) {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return 0, errs.Wrap("s.postRepo.Get", err)
	}

	var likes int

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if likes, err = s.likeRepo.Like(ctx, postID, userID); err != nil {
			return errs.Wrap("s.likeRepo.Like", err)
		}

		e, ok := likeEvent(p, userID)
		if !ok {
			return nil
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, e))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return likes, nil
}

func (s *PostService) Unlike(ctx context.Context, userID, postID uuid.UUID) (int, error) {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return 0, errs.Wrap("s.postRepo.Get", err)
	}

	var likes int

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if likes, err = s.likeRepo.Unlike(ctx, postID, userID); err != nil {
			return errs.Wrap("s.likeRepo.Unlike", err)
		}

		e, ok := likeEvent(p, userID)
		if !ok {
			return nil
		}

		return errs.Wrap("s.notifier.Retract", s.notifier.Retract(ctx, e))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return likes, nil
}

// likeEvent is the like of p by userID for its author's notifications;
// nobody is notified about liking their own post.
func likeEvent(p *post.Post, userID uuid.UUID) (Event, bool) {
	if p.AuthorID() == userID {
		return Event{}, false
	}

	return Event{Kind: EventLike, RecipientID: p.AuthorID(), ActorID: userID, PostID: p.ID()}, true
}

// ListWall pages a wall by keyset: cursor is the ID of the last post of
// the previous page, so new posts never shift the pages.
func (s *PostService) ListWall(ctx context.Context, viewerID, wallID, cursor uuid.UUID, size int) (Page, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra post tells whether there is a next page.
	posts, err := s.postRepo.ListByWall(ctx, wallID, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListByWall", err)
	}

	total, err := s.postRepo.CountByWall(ctx, wallID)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.CountByWall", err)
	}

	page := Page{Posts: posts, Total: total}

	if len(posts) > size {
		page.Posts = posts[:size]
		page.NextCursor = posts[size-1].ID()
	}

	if page.Liked, err = s.liked(ctx, viewerID, page.Posts); err != nil {
		return Page{}, errs.Wrap("s.liked", err)
	}

	return page, nil
}

// Feed pages the posts written by several authors together, on their own
// walls and on others', newest first. Total is not counted: a feed is read
// page by page, not as a whole.
func (s *PostService) Feed(ctx context.Context, viewerID uuid.UUID, authorIDs []uuid.UUID, cursor uuid.UUID, size int) (Page, error) {
	if len(authorIDs) > MaxFeedAuthors {
		return Page{}, ErrTooManyAuthors
	}

	if len(authorIDs) == 0 {
		return Page{}, nil
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	posts, err := s.postRepo.ListByAuthors(ctx, authorIDs, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListByAuthors", err)
	}

	page := Page{Posts: posts}

	if len(posts) > size {
		page.Posts = posts[:size]
		page.NextCursor = posts[size-1].ID()
	}

	if page.Liked, err = s.liked(ctx, viewerID, page.Posts); err != nil {
		return Page{}, errs.Wrap("s.liked", err)
	}

	return page, nil
}

// liked tells which posts the viewer likes; nothing for anonymous viewers.
func (s *PostService) liked(ctx context.Context, viewerID uuid.UUID, posts []*post.Post) (map[uuid.UUID]bool, error) {
	if viewerID == uuid.Nil || len(posts) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		ids[i] = p.ID()
	}

	liked, err := s.likeRepo.Liked(ctx, viewerID, ids)
	if err != nil {
		return nil, errs.Wrap("s.likeRepo.Liked", err)
	}

	return liked, nil
}
