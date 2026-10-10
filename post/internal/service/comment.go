package service

import (
	"bytes"
	"context"
	"slices"
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
	// LastByPosts returns the newest limit comments of each of the posts,
	// in no particular order; those of archived posts are not told.
	LastByPosts(ctx context.Context, postIDs []uuid.UUID, limit int) ([]*post.Comment, error)
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
	// Likers is LikeRepository.Likers for a comment.
	Likers(ctx context.Context, commentID uuid.UUID, after *LikerCursor, limit int) ([]Liker, error)
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

// CommentInput is a comment by AuthorID under the post PostID: an answer
// to the comment ParentID under the same post, as openvk's "Ответить",
// unless that is uuid.Nil. FromWall: it speaks for the community whose
// wall the post is on, which the caller has checked the author may.
type CommentInput struct {
	AuthorID uuid.UUID
	PostID   uuid.UUID
	ParentID uuid.UUID
	Content  post.CommentContent
	FromWall bool
	// Mentions are the people the text names, who hear of it.
	Mentions post.Mentions
}

// Create comments on a post with text, photos or both; media keeps the
// photos, which the caller has checked are the author's. The author of
// the post hears of it, the author of the comment answered of the
// answer, and the people it names of that.
func (s *CommentService) Create(ctx context.Context, in CommentInput) (*post.Comment, error) {
	var parent *post.Comment
	if in.ParentID != uuid.Nil {
		var err error
		if parent, err = s.commentRepo.Get(ctx, in.ParentID); err != nil {
			return nil, errs.Wrap("s.commentRepo.Get", err)
		}
	}

	// The post tells whom to notify; Save still reports a post deleted
	// in between.
	p, err := s.postRepo.Get(ctx, in.PostID)
	if err != nil {
		return nil, errs.Wrap("s.postRepo.Get", err)
	}

	if err = s.checkOpen(ctx, p, in.AuthorID); err != nil {
		return nil, err
	}

	var c *post.Comment
	if parent == nil {
		c, err = post.NewComment(in.PostID, in.AuthorID, in.Content, s.now())
	} else {
		c, err = post.NewReply(in.PostID, parent, in.AuthorID, in.Content, s.now())
	}
	if err != nil {
		return nil, errs.Wrap("post.NewComment", err)
	}

	if in.FromWall {
		if c, err = c.ForWall(p); err != nil {
			return nil, errs.Wrap("c.ForWall", err)
		}
	}

	if err = s.save(ctx, p, c, commentEvents(p, c, parent, in.Mentions)); err != nil {
		return nil, errs.Wrap("s.save", err)
	}

	return c, nil
}

// checkOpen tells whether authorID may comment p at all: not a post put
// away from them (post.ErrNotFound), nor one whose comments the owner of
// its wall has closed (post.ErrCommentsClosed), for everyone alike.
func (s *CommentService) checkOpen(ctx context.Context, p *post.Post, authorID uuid.UUID) error {
	away, err := hidden(ctx, s.moderators, p, authorID)
	if err != nil {
		return errs.Wrap("hidden", err)
	}

	if away {
		return post.ErrNotFound
	}

	if p.CommentsClosed() {
		return post.ErrCommentsClosed
	}

	return nil
}

// save stores comment c on p with its photos and notifications in one
// transaction.
func (s *CommentService) save(ctx context.Context, p *post.Post, c *post.Comment, events []Event) error {
	return errs.Wrap("s.tx.WithinTx", s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.commentRepo.Save(ctx, c); err != nil {
			return errs.Wrap("s.commentRepo.Save", err)
		}

		if photos := c.Photos(); len(photos) > 0 {
			if err := s.photos.Publish(ctx, c.AuthorID(), photos); err != nil {
				return errs.Wrap("s.photos.Publish", err)
			}
		}

		for _, e := range events {
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
// Likers pages who like a comment, newest first; post.ErrCommentNotFound
// for a missing comment.
func (s *CommentService) Likers(ctx context.Context, commentID uuid.UUID, cursor *LikerCursor, size int) (LikerPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	c, err := s.commentRepo.Get(ctx, commentID)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.commentRepo.Get", err)
	}

	found, err := s.likeRepo.Likers(ctx, commentID, cursor, size+1)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.likeRepo.Likers", err)
	}

	return likerPage(found, size, c.Likes()), nil
}

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

		// Nobody is notified about liking their own comment, nor about a
		// comment that speaks for a community (post.Comment.Notified).
		recipient := c.Notified()
		if recipient == uuid.Nil || recipient == userID {
			return nil
		}

		e := Event{Kind: EventCommentLike, RecipientID: recipient, ActorID: userID, PostID: c.PostID(), CommentID: commentID}

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
// already. Nobody hears of their own comments, and a post or a comment
// that speaks for a community notifies nobody (Notified). A comment for
// the community comes from the community, not from its author. The
// people it names hear of that, unless they heard of the comment already.
func commentEvents(p *post.Post, c, parent *post.Comment, mentions post.Mentions) []Event {
	var events []Event

	actor := c.AuthorID()
	if c.FromWall() {
		actor = p.WallID()
	}

	answered := uuid.Nil
	if parent != nil {
		answered = parent.Notified()
	}

	if answered != uuid.Nil && answered != c.AuthorID() {
		events = append(events, Event{Kind: EventCommentReply, RecipientID: answered, ActorID: actor, PostID: p.ID(), CommentID: c.ID()})
	}

	author := p.Notified()
	if author != uuid.Nil && author != c.AuthorID() && author != answered {
		events = append(events, Event{Kind: EventComment, RecipientID: author, ActorID: actor, PostID: p.ID(), CommentID: c.ID()})
	}

	for _, id := range mentions.Heard(c.AuthorID(), answered, author) {
		events = append(events, Event{Kind: EventCommentMention, RecipientID: id, ActorID: actor, PostID: p.ID(), CommentID: c.ID()})
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

// LastComments is how many comments openvk's "Микроблог" shows under a
// post of a wall.
const LastComments = 3

// Last returns the newest LastComments comments of each of the posts,
// oldest first, as "Микроблог" shows them under the posts of a wall, and
// which of them the viewer likes (viewerID is uuid.Nil for an anonymous
// one). Archived posts tell none: only the owner of the wall reads them.
func (s *CommentService) Last(ctx context.Context, viewerID uuid.UUID, postIDs []uuid.UUID) (CommentPage, error) {
	if len(postIDs) > MaxPageSize {
		return CommentPage{}, ErrTooManyIDs
	}

	if len(postIDs) == 0 {
		return CommentPage{}, nil
	}

	comments, err := s.commentRepo.LastByPosts(ctx, postIDs, LastComments)
	if err != nil {
		return CommentPage{}, errs.Wrap("s.commentRepo.LastByPosts", err)
	}

	// UUIDv7 sort by time: the comments of a post read as a conversation.
	slices.SortFunc(comments, func(a, b *post.Comment) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(ai[:], bi[:])
	})

	page := CommentPage{Comments: comments}
	if viewerID == uuid.Nil || len(comments) == 0 {
		return page, nil
	}

	ids := make([]uuid.UUID, len(comments))
	for i, c := range comments {
		ids[i] = c.ID()
	}

	if page.Liked, err = s.likeRepo.Liked(ctx, viewerID, ids); err != nil {
		return CommentPage{}, errs.Wrap("s.likeRepo.Liked", err)
	}

	return page, nil
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
