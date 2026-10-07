package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// PostEraser deletes the posts a user wrote and those on their wall,
// returning their IDs.
type PostEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// ReactionEraser deletes a user's likes or comments on any posts.
type ReactionEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// PurgeService erases what post keeps of a user whose page was deleted for
// good.
type PurgeService struct {
	posts    PostEraser
	likes    ReactionEraser
	comments ReactionEraser
	tx       TxManager
	notifier Notifier
}

// PurgeDeps are what PurgeService works with.
type PurgeDeps struct {
	Posts    PostEraser
	Likes    ReactionEraser
	Comments ReactionEraser
	Tx       TxManager
	Notifier Notifier
}

func NewPurgeService(d PurgeDeps) *PurgeService {
	return &PurgeService{
		posts:    d.Posts,
		likes:    d.Likes,
		comments: d.Comments,
		tx:       d.Tx,
		notifier: d.Notifier,
	}
}

// PurgeUser deletes, in one transaction, the user's likes and comments on
// others' posts, the posts they wrote and those on their wall (others' on
// it included, as a deleted post takes its comments). The notifications
// about the deleted posts go with them, as when a post is deleted.
func (s *PurgeService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.likes.DeleteByUser(ctx, userID); err != nil {
			return errs.Wrap("s.likes.DeleteByUser", err)
		}

		if err := s.comments.DeleteByUser(ctx, userID); err != nil {
			return errs.Wrap("s.comments.DeleteByUser", err)
		}

		deleted, err := s.posts.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.posts.DeleteByUser", err)
		}

		for _, id := range deleted {
			if err = s.notifier.ForgetPost(ctx, id); err != nil {
				return errs.Wrap("s.notifier.ForgetPost", err)
			}
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
