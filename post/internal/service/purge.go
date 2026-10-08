package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// PostEraser deletes the posts a user wrote and those on their wall,
// returning them.
type PostEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.Post, error)
}

// ReactionEraser deletes a user's likes or comments on any posts.
type ReactionEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// CommentEraser also deletes, returning them, the comments on the posts a
// user wrote and on those on their wall, which others wrote too.
type CommentEraser interface {
	ReactionEraser
	DeleteOnPostsOf(ctx context.Context, userID uuid.UUID) ([]*post.Comment, error)
}

// SuggestionEraser deletes what a user suggested for communities' walls,
// returning the IDs; media erases their photos with the rest of the user.
type SuggestionEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// TopicEraser deletes the messages a user wrote in topics and the topics
// they started, returning the messages of others those took with them.
type TopicEraser interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.TopicMessage, error)
}

// PurgeService erases what post keeps of a user whose page was deleted for
// good.
type PurgeService struct {
	posts        PostEraser
	likes        ReactionEraser
	commentLikes ReactionEraser
	comments     CommentEraser
	suggestions  SuggestionEraser
	topics       TopicEraser
	tx           TxManager
	notifier     Notifier
	photos       PhotoKeeper
}

// PurgeDeps are what PurgeService works with.
type PurgeDeps struct {
	Posts        PostEraser
	Likes        ReactionEraser
	CommentLikes ReactionEraser
	Comments     CommentEraser
	Suggestions  SuggestionEraser
	Topics       TopicEraser
	Tx           TxManager
	Notifier     Notifier
	Photos       PhotoKeeper
}

func NewPurgeService(d PurgeDeps) *PurgeService {
	return &PurgeService{
		posts:        d.Posts,
		likes:        d.Likes,
		commentLikes: d.CommentLikes,
		comments:     d.Comments,
		suggestions:  d.Suggestions,
		topics:       d.Topics,
		tx:           d.Tx,
		notifier:     d.Notifier,
		photos:       d.Photos,
	}
}

// PurgeUser deletes, in one transaction, the user's likes and comments on
// others' posts, what they suggested, their topics and the messages they
// wrote in topics, the posts they wrote and those on
// their wall (others' on
// it included, as a deleted post takes its comments). The notifications
// about the deleted posts and suggestions and the photos of the posts go
// with them, as when a post is deleted; the user's own photos media erases itself.
func (s *PurgeService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.likes.DeleteByUser(ctx, userID); err != nil {
			return errs.Wrap("s.likes.DeleteByUser", err)
		}

		if err := s.commentLikes.DeleteByUser(ctx, userID); err != nil {
			return errs.Wrap("s.commentLikes.DeleteByUser", err)
		}

		if err := s.comments.DeleteByUser(ctx, userID); err != nil {
			return errs.Wrap("s.comments.DeleteByUser", err)
		}

		suggestions, err := s.suggestions.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.suggestions.DeleteByUser", err)
		}

		// The admins heard about them.
		for _, id := range suggestions {
			if err = s.notifier.ForgetPost(ctx, id); err != nil {
				return errs.Wrap("s.notifier.ForgetPost", err)
			}
		}

		// Others' messages in the user's topics go with the topics, and
		// their photos with them.
		messages, err := s.topics.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.topics.DeleteByUser", err)
		}

		if err = deleteMessagePhotos(ctx, s.photos, messages); err != nil {
			return errs.Wrap("deleteMessagePhotos", err)
		}

		// Others' comments on the posts about to go: their photos go too.
		comments, err := s.comments.DeleteOnPostsOf(ctx, userID)
		if err != nil {
			return errs.Wrap("s.comments.DeleteOnPostsOf", err)
		}

		if err = deleteCommentPhotos(ctx, s.photos, comments); err != nil {
			return errs.Wrap("deleteCommentPhotos", err)
		}

		deleted, err := s.posts.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.posts.DeleteByUser", err)
		}

		for _, p := range deleted {
			if err = s.notifier.ForgetPost(ctx, p.ID()); err != nil {
				return errs.Wrap("s.notifier.ForgetPost", err)
			}

			// Others' posts on the wall; the user's own photos media
			// erases with the rest of the user.
			if ids := p.Content().Photos(); len(ids) > 0 && p.AuthorID() != userID {
				if err = s.photos.Delete(ctx, p.AuthorID(), ids); err != nil {
					return errs.Wrap("s.photos.Delete", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
