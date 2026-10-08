package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type CommentRepository interface {
	// Save returns post.ErrNotFound when the post is gone and
	// post.ErrCommentNotFound when the comment it answers is; storage
	// counts the comment in the post's counters.
	Save(ctx context.Context, c *post.Comment) error
	// Get and Delete return post.ErrCommentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Comment, error)
	// GetMany returns the comments that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ListByPost returns up to limit comments of a post, oldest first, with
	// IDs above after (uuid.Nil: from the first).
	ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error)
}

// CommentLikeRepository keeps the likes of comments as LikeRepository
// does those of posts.
type CommentLikeRepository interface {
	// Like and Unlike are idempotent and return how many likes the
	// comment has now; post.ErrCommentNotFound for a missing comment.
	Like(ctx context.Context, commentID, userID uuid.UUID) (int, error)
	Unlike(ctx context.Context, commentID, userID uuid.UUID) (int, error)
	// Liked returns which of commentIDs the user likes.
	Liked(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// PostReader finds the post a comment belongs to.
type PostReader interface {
	// Get returns post.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
}

// CommentPage is one screen of the comments of a post. NextCursor is
// uuid.Nil on the last page. Liked marks the comments the viewer likes;
// it is empty for anonymous viewers.
type CommentPage struct {
	Comments   []*post.Comment
	Liked      map[uuid.UUID]bool
	NextCursor uuid.UUID
	Total      int
}

type CommentService struct {
	postRepo    PostReader
	commentRepo CommentRepository
	likeRepo    CommentLikeRepository
	tx          TxManager
	notifier    Notifier
	photos      PhotoKeeper
	moderators  WallModerators
	now         func() time.Time
}

// CommentDeps are what CommentService works with.
type CommentDeps struct {
	Posts      PostReader
	Comments   CommentRepository
	Likes      CommentLikeRepository
	Tx         TxManager
	Notifier   Notifier
	Photos     PhotoKeeper
	Moderators WallModerators
}

func NewCommentService(d CommentDeps) *CommentService {
	return &CommentService{
		postRepo:    d.Posts,
		commentRepo: d.Comments,
		likeRepo:    d.Likes,
		tx:          d.Tx,
		notifier:    d.Notifier,
		photos:      d.Photos,
		moderators:  d.Moderators,
		now:         time.Now,
	}
}

// Create comments on a post with text, photos or both; media keeps the
// photos, which the caller has checked are the author's.
func (s *CommentService) Create(ctx context.Context, authorID, postID uuid.UUID, content post.CommentContent) (*post.Comment, error) {
	// The post tells whom to notify; Save still reports a post deleted
	// in between.
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return nil, errs.Wrap("s.postRepo.Get", err)
	}

	c, err := post.NewComment(postID, authorID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewComment", err)
	}

	if err = s.save(ctx, p, c, nil); err != nil {
		return nil, errs.Wrap("s.save", err)
	}

	return c, nil
}

// ReplyInput is an answer by AuthorID to the comment ParentID under the
// post PostID.
type ReplyInput struct {
	AuthorID uuid.UUID
	PostID   uuid.UUID
	ParentID uuid.UUID
	Content  post.CommentContent
}

// Reply answers a comment under its post, as openvk's "Ответить": the
// author of the comment hears of the answer.
func (s *CommentService) Reply(ctx context.Context, in ReplyInput) (*post.Comment, error) {
	parent, err := s.commentRepo.Get(ctx, in.ParentID)
	if err != nil {
		return nil, errs.Wrap("s.commentRepo.Get", err)
	}

	p, err := s.postRepo.Get(ctx, in.PostID)
	if err != nil {
		return nil, errs.Wrap("s.postRepo.Get", err)
	}

	c, err := post.NewReply(in.PostID, parent, in.AuthorID, in.Content, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewReply", err)
	}

	if err = s.save(ctx, p, c, parent); err != nil {
		return nil, errs.Wrap("s.save", err)
	}

	return c, nil
}

// save stores comment c on p, which answers parent (nil for none), with
// its photos and notifications in one transaction.
func (s *CommentService) save(ctx context.Context, p *post.Post, c, parent *post.Comment) error {
	return errs.Wrap("s.tx.WithinTx", s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.commentRepo.Save(ctx, c); err != nil {
			return errs.Wrap("s.commentRepo.Save", err)
		}

		if photos := c.Photos(); len(photos) > 0 {
			if err := s.photos.Publish(ctx, c.AuthorID(), photos); err != nil {
				return errs.Wrap("s.photos.Publish", err)
			}
		}

		for _, e := range commentEvents(p, c, parent) {
			if err := s.notifier.Notify(ctx, e); err != nil {
				return errs.Wrap("s.notifier.Notify", err)
			}
		}

		return nil
	}))
}

// Delete removes a comment with its photos and likes; every notification
// of it goes, those of the reply it was and of its likes too.
func (s *CommentService) Delete(ctx context.Context, requesterID, commentID uuid.UUID) error {
	c, err := s.commentRepo.Get(ctx, commentID)
	if err != nil {
		return errs.Wrap("s.commentRepo.Get", err)
	}

	p, err := s.postRepo.Get(ctx, c.PostID())
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, p.WallID(), requesterID, c.CanBeDeletedBy(requesterID, p))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotDeleteComment
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.commentRepo.Delete(ctx, commentID); err != nil {
			return errs.Wrap("s.commentRepo.Delete", err)
		}

		if err := deleteCommentPhotos(ctx, s.photos, []*post.Comment{c}); err != nil {
			return errs.Wrap("deleteCommentPhotos", err)
		}

		return errs.Wrap("s.notifier.ForgetComment", s.notifier.ForgetComment(ctx, commentID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// Like is idempotent, as for posts; the author of the comment hears of it.
// It returns how many likes the comment has now.
func (s *CommentService) Like(ctx context.Context, userID, commentID uuid.UUID) (int, error) {
	return s.setLike(ctx, userID, commentID, true)
}

func (s *CommentService) Unlike(ctx context.Context, userID, commentID uuid.UUID) (int, error) {
	return s.setLike(ctx, userID, commentID, false)
}

func (s *CommentService) setLike(ctx context.Context, userID, commentID uuid.UUID, like bool) (int, error) {
	c, err := s.commentRepo.Get(ctx, commentID)
	if err != nil {
		return 0, errs.Wrap("s.commentRepo.Get", err)
	}

	set, tell := s.likeRepo.Unlike, s.notifier.Retract
	if like {
		set, tell = s.likeRepo.Like, s.notifier.Notify
	}

	var likes int

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if likes, err = set(ctx, commentID, userID); err != nil {
			return errs.Wrap("set", err)
		}

		// Nobody is notified about liking their own comment.
		if c.AuthorID() == userID {
			return nil
		}

		e := Event{Kind: EventCommentLike, RecipientID: c.AuthorID(), ActorID: userID, PostID: c.PostID(), CommentID: commentID}

		return errs.Wrap("tell", tell(ctx, e))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return likes, nil
}

// deleteCommentPhotos records, in the transaction of ctx, that media
// deletes the photos of deleted comments.
func deleteCommentPhotos(ctx context.Context, photos PhotoKeeper, comments []*post.Comment) error {
	for _, c := range comments {
		if ids := c.Photos(); len(ids) > 0 {
			if err := photos.Delete(ctx, c.AuthorID(), ids); err != nil {
				return errs.Wrap("photos.Delete", err)
			}
		}
	}

	return nil
}

// commentEvents are the notifications of comment c on p, which answers
// parent (nil for none): the author of parent hears of the answer, the
// author of the post of a new comment, unless they heard of the answer
// already. Nobody hears of their own comments.
func commentEvents(p *post.Post, c, parent *post.Comment) []Event {
	var events []Event

	if parent != nil && parent.AuthorID() != c.AuthorID() {
		events = append(events, Event{Kind: EventCommentReply, RecipientID: parent.AuthorID(), ActorID: c.AuthorID(), PostID: p.ID(), CommentID: c.ID()})
	}

	// A post that speaks for a community notifies nobody (post.Notified).
	author := p.Notified()
	if author != uuid.Nil && author != c.AuthorID() && (parent == nil || parent.AuthorID() != author) {
		events = append(events, Event{Kind: EventComment, RecipientID: author, ActorID: c.AuthorID(), PostID: p.ID(), CommentID: c.ID()})
	}

	return events
}

// GetMany returns the comments with these IDs that exist, at most
// MaxPageSize, in no particular order.
func (s *CommentService) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error) {
	if len(ids) > MaxPageSize {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	comments, err := s.commentRepo.GetMany(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.commentRepo.GetMany", err)
	}

	return comments, nil
}

// List pages the comments of a post oldest first, as a conversation reads:
// cursor is the ID of the last comment of the previous page. viewerID is
// uuid.Nil for an anonymous viewer.
func (s *CommentService) List(ctx context.Context, viewerID, postID, cursor uuid.UUID, size int) (CommentPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// The post tells the total and turns comments of a deleted post into
	// post.ErrNotFound instead of an empty page.
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return CommentPage{}, errs.Wrap("s.postRepo.Get", err)
	}

	// One extra comment tells whether there is a next page.
	comments, err := s.commentRepo.ListByPost(ctx, postID, cursor, size+1)
	if err != nil {
		return CommentPage{}, errs.Wrap("s.commentRepo.ListByPost", err)
	}

	page := CommentPage{Comments: comments, Total: p.Counters().Comments}

	if len(comments) > size {
		page.Comments = comments[:size]
		page.NextCursor = comments[size-1].ID()
	}

	if viewerID != uuid.Nil && len(page.Comments) > 0 {
		ids := make([]uuid.UUID, len(page.Comments))
		for i, c := range page.Comments {
			ids[i] = c.ID()
		}

		if page.Liked, err = s.likeRepo.Liked(ctx, viewerID, ids); err != nil {
			return CommentPage{}, errs.Wrap("s.likeRepo.Liked", err)
		}
	}

	return page, nil
}
