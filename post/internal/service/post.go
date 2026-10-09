package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxFeedAuthors bounds one news feed: the people a user follows plus
	// the user, and as many clubs.
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
	// Update saves what an edited post says; post.ErrNotFound.
	Update(ctx context.Context, p *post.Post) error
	// ListByWall returns up to limit posts on a wall that filter picks,
	// newest first, with IDs below before (uuid.Nil: from the newest). Of
	// all posts the pinned one is left out: the wall shows it first.
	ListByWall(ctx context.Context, wallID uuid.UUID, filter post.WallFilter, before uuid.UUID, limit int) ([]*post.Post, error)
	CountByWall(ctx context.Context, wallID uuid.UUID, filter post.WallFilter) (int, error)
	// ListFeed is ListByWall for the posts written by several authors, on
	// whichever walls, but for those that speak for a wall, together with
	// all the posts on several walls.
	ListFeed(ctx context.Context, sources FeedSources, before uuid.UUID, limit int) ([]*post.Post, error)
}

// FeedSources are what a news feed shows: the posts of Authors (those
// that speak for a community are the community's, not theirs) and every
// post on Walls, the clubs the reader is in.
type FeedSources struct {
	Authors []uuid.UUID
	Walls   []uuid.UUID
}

// WallModerators tells who keeps a wall clean besides its owner: the
// admins of a community, whom the club service knows. It says no for the
// wall of a user.
type WallModerators interface {
	CanModerate(ctx context.Context, wallID, userID uuid.UUID) (bool, error)
}

// PostCommentEraser deletes the comments of a post about to be deleted,
// returning them, in the transaction of ctx; a comment written meanwhile
// fails rather than goes unseen with the post.
type PostCommentEraser interface {
	DeleteByPost(ctx context.Context, postID uuid.UUID) ([]*post.Comment, error)
}

// PinRepository keeps the post each wall shows first.
type PinRepository interface {
	// Pin pins a post on its wall instead of the one pinned before;
	// post.ErrNotFound for a missing post.
	Pin(ctx context.Context, id uuid.UUID) error
	// Unpin is fine for a post that is not pinned.
	Unpin(ctx context.Context, id uuid.UUID) error
	// Pinned returns post.ErrNotFound when nothing is pinned on the wall.
	Pinned(ctx context.Context, wallID uuid.UUID) (*post.Post, error)
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
	postRepo    PostRepository
	pinRepo     PinRepository
	commentRepo PostCommentEraser
	likeRepo    LikeRepository
	tx          TxManager
	notifier    Notifier
	photos      PhotoKeeper
	moderators  WallModerators
	now         func() time.Time
}

// PostDeps are what PostService works with.
type PostDeps struct {
	Posts      PostRepository
	Pins       PinRepository
	Comments   PostCommentEraser
	Likes      LikeRepository
	Tx         TxManager
	Notifier   Notifier
	Photos     PhotoKeeper
	Moderators WallModerators
}

func NewPostService(d PostDeps) *PostService {
	return &PostService{
		postRepo:    d.Posts,
		pinRepo:     d.Pins,
		commentRepo: d.Comments,
		likeRepo:    d.Likes,
		tx:          d.Tx,
		notifier:    d.Notifier,
		photos:      d.Photos,
		moderators:  d.Moderators,
		now:         time.Now,
	}
}

// CreateInput is a post to write: by AuthorID on the wall of WallID, the
// author's or someone else's. ClubWall tells that the wall is a
// community's, whose owner is nobody to notify; FromWall that the post
// speaks for that community, Signed that it names its author too.
// AvatarUpdate makes it the post of the new photo of the wall's owner (see
// post.NewAvatarUpdate).
type CreateInput struct {
	WallID   uuid.UUID
	AuthorID uuid.UUID
	Content  post.Content
	ClubWall bool
	FromWall bool
	Signed   bool

	AvatarUpdate bool
}

// Create writes a post; the owner of a user's wall hears about posts by
// others, and media keeps the photos. The caller has checked that the
// author may write there, on their own behalf or the community's, and
// that the photos are the author's: post knows neither users nor clubs.
func (s *PostService) Create(ctx context.Context, in CreateInput) (*post.Post, error) {
	wallID, authorID, content := in.WallID, in.AuthorID, in.Content

	var (
		p   *post.Post
		err error
	)
	switch {
	case in.AvatarUpdate:
		p, err = post.NewAvatarUpdate(wallID, authorID, content, s.now())
	case in.FromWall:
		p, err = post.NewFromWall(wallID, authorID, content, in.Signed, s.now())
	default:
		p, err = post.New(wallID, authorID, content, s.now())
	}
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

		if p.OnOwnWall() || in.ClubWall {
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

		recipient := shared.Notified()
		if recipient == uuid.Nil || recipient == authorID {
			return nil
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, Event{Kind: EventRepost, RecipientID: recipient, ActorID: authorID, PostID: repost.ID()}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return repost, nil
}

// Delete removes a post with its comments and all their photos, whoever
// deletes it: the author, the owner of the wall or, on a community's,
// its admins. A deleted repost takes
// its notification with it, since the notification is about the repost
// (ForgetPost).
func (s *PostService) Delete(ctx context.Context, requesterID, postID uuid.UUID) error {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, p.WallID(), requesterID, p.CanBeDeletedBy(requesterID))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotDelete
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		comments, err := s.commentRepo.DeleteByPost(ctx, postID)
		if err != nil {
			return errs.Wrap("s.commentRepo.DeleteByPost", err)
		}

		if err = deleteCommentPhotos(ctx, s.photos, comments); err != nil {
			return errs.Wrap("deleteCommentPhotos", err)
		}

		if err = s.postRepo.Delete(ctx, postID); err != nil {
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

// Edit changes what a post says, by its author within a week (see
// post.Edited). Media keeps the photos added and deletes those taken away.
// It returns the edited post and whether the author likes it.
func (s *PostService) Edit(ctx context.Context, requesterID, postID uuid.UUID, text post.Text, photos post.Photos) (*post.Post, bool, error) {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return nil, false, errs.Wrap("s.postRepo.Get", err)
	}

	edited, err := p.Edited(requesterID, text, photos, s.now())
	if err != nil {
		return nil, false, errs.Wrap("p.Edited", err)
	}

	added, removed := diffPhotos(p.Content().Photos(), photos)

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.postRepo.Update(ctx, edited); err != nil {
			return errs.Wrap("s.postRepo.Update", err)
		}

		if len(added) > 0 {
			if err := s.photos.Publish(ctx, p.AuthorID(), added); err != nil {
				return errs.Wrap("s.photos.Publish", err)
			}
		}

		if len(removed) > 0 {
			return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, p.AuthorID(), removed))
		}

		return nil
	})
	if err != nil {
		return nil, false, errs.Wrap("s.tx.WithinTx", err)
	}

	liked, err := s.liked(ctx, requesterID, []*post.Post{edited})
	if err != nil {
		return nil, false, errs.Wrap("s.liked", err)
	}

	return edited, liked[edited.ID()], nil
}

// diffPhotos tells which photos an edit adds and which it takes away.
func diffPhotos(before, after post.Photos) (added, removed []uuid.UUID) {
	for _, id := range after {
		if !slices.Contains(before, id) {
			added = append(added, id)
		}
	}

	for _, id := range before {
		if !slices.Contains(after, id) {
			removed = append(removed, id)
		}
	}

	return added, removed
}

// Pin shows a post first on its wall, instead of the one pinned before;
// only the owner of the wall pins, or the admins of a community.
func (s *PostService) Pin(ctx context.Context, requesterID, postID uuid.UUID) error {
	return s.setPinned(ctx, requesterID, postID, s.pinRepo.Pin)
}

// Unpin returns a pinned post to its place on the wall.
func (s *PostService) Unpin(ctx context.Context, requesterID, postID uuid.UUID) error {
	return s.setPinned(ctx, requesterID, postID, s.pinRepo.Unpin)
}

func (s *PostService) setPinned(ctx context.Context, requesterID, postID uuid.UUID, set func(context.Context, uuid.UUID) error) error {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, p.WallID(), requesterID, p.CanBePinnedBy(requesterID))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotPin
	}

	return errs.Wrap("set", set(ctx, postID))
}

// moderates tells whether userID may act on a wall: allowed by the rules
// of post itself, or else as an admin of the community the wall is.
// Asking the club service only then spares it every call by an owner.
func moderates(ctx context.Context, moderators WallModerators, wallID, userID uuid.UUID, allowed bool) (bool, error) {
	if allowed {
		return true, nil
	}

	ok, err := moderators.CanModerate(ctx, wallID, userID)
	if err != nil {
		return false, errs.Wrap("moderators.CanModerate", err)
	}

	return ok, nil
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
// nobody is notified about liking their own post, nor about a post that
// speaks for a community.
func likeEvent(p *post.Post, userID uuid.UUID) (Event, bool) {
	recipient := p.Notified()
	if recipient == uuid.Nil || recipient == userID {
		return Event{}, false
	}

	return Event{Kind: EventLike, RecipientID: recipient, ActorID: userID, PostID: p.ID()}, true
}

// ListWall pages the posts of a wall filter picks by keyset: cursor is
// the ID of the last post of the previous page, so new posts never shift
// the pages. Of all posts the pinned one comes first on the first page,
// on top of size others; the other tabs show it in its place.
func (s *PostService) ListWall(ctx context.Context, viewerID, wallID uuid.UUID, filter post.WallFilter, cursor uuid.UUID, size int) (Page, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra post tells whether there is a next page.
	posts, err := s.postRepo.ListByWall(ctx, wallID, filter, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListByWall", err)
	}

	var pinned *post.Post
	if cursor == uuid.Nil && filter == post.WallAll {
		if pinned, err = s.pinRepo.Pinned(ctx, wallID); err != nil && !errors.Is(err, post.ErrNotFound) {
			return Page{}, errs.Wrap("s.pinRepo.Pinned", err)
		}
	}

	total, err := s.postRepo.CountByWall(ctx, wallID, filter)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.CountByWall", err)
	}

	page := Page{Posts: posts, Total: total}

	if len(posts) > size {
		page.Posts = posts[:size]
		page.NextCursor = posts[size-1].ID()
	}

	if pinned != nil {
		page.Posts = append([]*post.Post{pinned}, page.Posts...)
	}

	if page.Liked, err = s.liked(ctx, viewerID, page.Posts); err != nil {
		return Page{}, errs.Wrap("s.liked", err)
	}

	return page, nil
}

// Feed pages the posts written by several authors together, on their own
// walls and on others', with every post on the walls of the reader's
// clubs, newest first. Total is not counted: a feed is read page by page,
// not as a whole.
func (s *PostService) Feed(ctx context.Context, viewerID uuid.UUID, sources FeedSources, cursor uuid.UUID, size int) (Page, error) {
	if len(sources.Authors) > MaxFeedAuthors || len(sources.Walls) > MaxFeedAuthors {
		return Page{}, ErrTooManyAuthors
	}

	if len(sources.Authors) == 0 && len(sources.Walls) == 0 {
		return Page{}, nil
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	posts, err := s.postRepo.ListFeed(ctx, sources, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListFeed", err)
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
