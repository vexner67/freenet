package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// A person comments remote posts at most commentLimit times in
// commentPeriod, as posts of the site.
const (
	commentLimit  = 20
	commentPeriod = time.Minute
)

// CommentPage is a page of the comments of a remote post, the oldest
// first; Total is how many it has.
type CommentPage struct {
	Comments []*fedi.RemoteComment
	// Next is the cursor of the next page, uuid.Nil after the last.
	Next  uuid.UUID
	Total int
}

// Comment keeps what userID writes under a remote post. The comments stay
// here: its server does not learn of them.
func (s *Service) Comment(ctx context.Context, userID, postID uuid.UUID, text string) (*fedi.RemoteComment, error) {
	c, err := fedi.NewRemoteComment(postID, userID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("fedi.NewRemoteComment", err)
	}

	ok, err := s.limiter.Allow(ctx, "comment:"+userID.String(), commentLimit, commentPeriod)
	if err != nil {
		return nil, errs.Wrap("s.limiter.Allow", err)
	}
	if !ok {
		return nil, fedi.ErrTooManyComments
	}

	if _, err = s.news.Get(ctx, postID); err != nil {
		return nil, errs.Wrap("s.news.Get", err)
	}

	if err = s.comments.AddComment(ctx, c); err != nil {
		return nil, errs.Wrap("s.comments.AddComment", err)
	}

	return c, nil
}

// Comments are the comments of a remote post, the oldest first, a page of
// limit after the cursor after.
func (s *Service) Comments(ctx context.Context, postID, after uuid.UUID, limit int) (CommentPage, error) {
	if _, err := s.news.Get(ctx, postID); err != nil {
		return CommentPage{}, errs.Wrap("s.news.Get", err)
	}

	comments, err := s.comments.Comments(ctx, postID, after, limit+1)
	if err != nil {
		return CommentPage{}, errs.Wrap("s.comments.Comments", err)
	}

	counts, err := s.comments.CommentCounts(ctx, []uuid.UUID{postID})
	if err != nil {
		return CommentPage{}, errs.Wrap("s.comments.CommentCounts", err)
	}

	page := CommentPage{Total: counts[postID]}
	if len(comments) > limit {
		comments = comments[:limit]
		page.Next = comments[limit-1].ID()
	}
	page.Comments = comments

	return page, nil
}

// DeleteComment takes away a comment of userID; fedi.ErrNotYourComment
// for that of another.
func (s *Service) DeleteComment(ctx context.Context, userID, id uuid.UUID) error {
	c, err := s.comments.Comment(ctx, id)
	if err != nil {
		return errs.Wrap("s.comments.Comment", err)
	}

	if c.AuthorID() != userID {
		return fedi.ErrNotYourComment
	}

	return errs.Wrap("s.comments.DeleteComment", s.comments.DeleteComment(ctx, id))
}
