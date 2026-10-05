package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type CommentRepository interface {
	// Save returns post.ErrNotFound when the post is gone; storage counts
	// the comment in the post's counters.
	Save(ctx context.Context, c *post.Comment) error
	// Get and Delete return post.ErrCommentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Comment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ListByPost returns up to limit comments of a post, oldest first, with
	// IDs above after (uuid.Nil: from the first).
	ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error)
}

// PostReader finds the post a comment belongs to.
type PostReader interface {
	// Get returns post.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
}

// CommentPage is one screen of the comments of a post. NextCursor is
// uuid.Nil on the last page.
type CommentPage struct {
	Comments   []*post.Comment
	NextCursor uuid.UUID
	Total      int
}

type CommentService struct {
	postRepo    PostReader
	commentRepo CommentRepository
	now         func() time.Time
}

func NewCommentService(postRepo PostReader, commentRepo CommentRepository) *CommentService {
	return &CommentService{
		postRepo:    postRepo,
		commentRepo: commentRepo,
		now:         time.Now,
	}
}

func (s *CommentService) Create(ctx context.Context, authorID, postID uuid.UUID, text post.CommentText) (*post.Comment, error) {
	c, err := post.NewComment(postID, authorID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewComment", err)
	}

	if err = s.commentRepo.Save(ctx, c); err != nil {
		return nil, errs.Wrap("s.commentRepo.Save", err)
	}

	return c, nil
}

func (s *CommentService) Delete(ctx context.Context, requesterID, commentID uuid.UUID) error {
	c, err := s.commentRepo.Get(ctx, commentID)
	if err != nil {
		return errs.Wrap("s.commentRepo.Get", err)
	}

	p, err := s.postRepo.Get(ctx, c.PostID())
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	if !c.CanBeDeletedBy(requesterID, p) {
		return post.ErrCannotDeleteComment
	}

	if err = s.commentRepo.Delete(ctx, commentID); err != nil {
		return errs.Wrap("s.commentRepo.Delete", err)
	}

	return nil
}

// List pages the comments of a post oldest first, as a conversation reads:
// cursor is the ID of the last comment of the previous page.
func (s *CommentService) List(ctx context.Context, postID, cursor uuid.UUID, size int) (CommentPage, error) {
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

	return page, nil
}
