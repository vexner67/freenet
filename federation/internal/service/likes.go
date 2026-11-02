package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// A person likes and unlikes remote posts at most likeLimit times in
// likePeriod, as posts of the site.
const (
	likeLimit  = 60
	likePeriod = time.Minute
)

// LikerPage is a page of who here like a remote post, the latest first;
// Total is how many do.
type LikerPage struct {
	UserIDs []uuid.UUID
	// Next is the cursor of the next page, uuid.Nil after the last.
	Next  uuid.UUID
	Total int
}

// Like puts the heart of userID on a remote post, telling how many like
// it then. The likes stay here: its server does not learn of them.
func (s *Service) Like(ctx context.Context, userID, postID uuid.UUID) (int, error) {
	if err := s.likable(ctx, userID, postID); err != nil {
		return 0, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return 0, errs.Wrap("uuid.NewV7", err)
	}

	if err = s.likes.Add(ctx, postID, Liker{ID: id, UserID: userID}, s.now()); err != nil {
		return 0, errs.Wrap("s.likes.Add", err)
	}

	return s.likeCount(ctx, postID)
}

// Unlike takes the heart of userID off a remote post.
func (s *Service) Unlike(ctx context.Context, userID, postID uuid.UUID) (int, error) {
	if err := s.likable(ctx, userID, postID); err != nil {
		return 0, err
	}

	if err := s.likes.Remove(ctx, postID, userID); err != nil {
		return 0, errs.Wrap("s.likes.Remove", err)
	}

	return s.likeCount(ctx, postID)
}

// likable: userID may like or unlike now, and the post is kept.
func (s *Service) likable(ctx context.Context, userID, postID uuid.UUID) error {
	ok, err := s.limiter.Allow(ctx, "like:"+userID.String(), likeLimit, likePeriod)
	if err != nil {
		return errs.Wrap("s.limiter.Allow", err)
	}
	if !ok {
		return fedi.ErrTooManyLikes
	}

	if _, err = s.news.Get(ctx, postID); err != nil {
		return errs.Wrap("s.news.Get", err)
	}

	return nil
}

func (s *Service) likeCount(ctx context.Context, postID uuid.UUID) (int, error) {
	likes, err := s.likes.Of(ctx, []uuid.UUID{postID}, uuid.Nil)
	if err != nil {
		return 0, errs.Wrap("s.likes.Of", err)
	}

	return likes[postID].Count, nil
}

// Likers are who here like a remote post, the latest first, a page of
// limit after the cursor before.
func (s *Service) Likers(ctx context.Context, postID, before uuid.UUID, limit int) (LikerPage, error) {
	if _, err := s.news.Get(ctx, postID); err != nil {
		return LikerPage{}, errs.Wrap("s.news.Get", err)
	}

	likers, err := s.likes.Likers(ctx, postID, before, limit+1)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.likes.Likers", err)
	}

	total, err := s.likeCount(ctx, postID)
	if err != nil {
		return LikerPage{}, err
	}

	page := LikerPage{Total: total}
	if len(likers) > limit {
		likers = likers[:limit]
		page.Next = likers[limit-1].ID
	}

	page.UserIDs = make([]uuid.UUID, len(likers))
	for i, l := range likers {
		page.UserIDs[i] = l.UserID
	}

	return page, nil
}
