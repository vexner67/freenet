package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// WallSuggestionEraser deletes, returning them, the posts suggested for a
// community's wall.
type WallSuggestionEraser interface {
	DeleteOnWall(ctx context.Context, wallID uuid.UUID) ([]*post.Suggestion, error)
}

// WallPostponedEraser deletes, returning them, the posts waiting for
// their time on a community's wall.
type WallPostponedEraser interface {
	DeleteOnWall(ctx context.Context, wallID uuid.UUID) ([]*post.Postponed, error)
}

// ClubTopicEraser deletes the topics of a community, returning their IDs
// and the messages they took with them.
type ClubTopicEraser interface {
	DeleteByClub(ctx context.Context, clubID uuid.UUID) ([]uuid.UUID, []*post.TopicMessage, error)
}

// OwnerAlbumEraser deletes the albums of an owner, returning the photos
// they took with them.
type OwnerAlbumEraser interface {
	DeleteByOwner(ctx context.Context, ownerID uuid.UUID) ([]*post.AlbumPhoto, error)
}

// MediaPurger tells media to erase everything it keeps under an ID: the
// photo and the backdrop of a community.
type MediaPurger interface {
	PhotoKeeper
	Purge(ctx context.Context, ownerID uuid.UUID) error
}

// ClubPurgeService erases what post keeps of a community that is gone.
type ClubPurgeService struct {
	posts       PostEraser
	comments    CommentEraser
	suggestions WallSuggestionEraser
	postponed   WallPostponedEraser
	topics      ClubTopicEraser
	albums      OwnerAlbumEraser
	sources     SourceRepository
	tx          TxManager
	notifier    Notifier
	media       MediaPurger
}

// ClubPurgeDeps are what ClubPurgeService works with.
type ClubPurgeDeps struct {
	Posts       PostEraser
	Comments    CommentEraser
	Suggestions WallSuggestionEraser
	Postponed   WallPostponedEraser
	Topics      ClubTopicEraser
	Albums      OwnerAlbumEraser
	Sources     SourceRepository
	Tx          TxManager
	Notifier    Notifier
	Media       MediaPurger
}

func NewClubPurgeService(d ClubPurgeDeps) *ClubPurgeService {
	return &ClubPurgeService{
		posts:       d.Posts,
		comments:    d.Comments,
		suggestions: d.Suggestions,
		postponed:   d.Postponed,
		topics:      d.Topics,
		albums:      d.Albums,
		sources:     d.Sources,
		tx:          d.Tx,
		notifier:    d.Notifier,
		media:       d.Media,
	}
}

// PurgeClub deletes, in one transaction, everything of a community: the
// posts on its wall with their comments, what was suggested and postponed
// for it, its topics with their messages and its albums with their
// photos. The notifications about them and their photos go with them, as
// when each is deleted, and so do the photo and the backdrop of the
// community. Idempotent.
func (s *ClubPurgeService) PurgeClub(ctx context.Context, clubID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Nobody reads the community any more, and its wall is not closed.
		if err := s.sources.Forget(ctx, clubID); err != nil {
			return errs.Wrap("s.sources.Forget", err)
		}

		if err := s.purgeWaiting(ctx, clubID); err != nil {
			return err
		}

		if err := s.purgeTopics(ctx, clubID); err != nil {
			return err
		}

		photos, err := s.albums.DeleteByOwner(ctx, clubID)
		if err != nil {
			return errs.Wrap("s.albums.DeleteByOwner", err)
		}

		if err = s.forgetAlbumPhotos(ctx, photos); err != nil {
			return err
		}

		if err = s.purgeWall(ctx, clubID); err != nil {
			return err
		}

		return errs.Wrap("s.media.Purge", s.media.Purge(ctx, clubID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// purgeWaiting deletes the posts suggested and postponed for the wall,
// whose photos media published already.
func (s *ClubPurgeService) purgeWaiting(ctx context.Context, clubID uuid.UUID) error {
	suggestions, err := s.suggestions.DeleteOnWall(ctx, clubID)
	if err != nil {
		return errs.Wrap("s.suggestions.DeleteOnWall", err)
	}

	for _, sg := range suggestions {
		// The admins heard about it.
		if err = s.notifier.ForgetPost(ctx, sg.ID()); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		if err = s.deletePhotos(ctx, sg.AuthorID(), sg.Content().Photos()); err != nil {
			return err
		}
	}

	postponed, err := s.postponed.DeleteOnWall(ctx, clubID)
	if err != nil {
		return errs.Wrap("s.postponed.DeleteOnWall", err)
	}

	for _, p := range postponed {
		if err = s.deletePhotos(ctx, p.AuthorID(), p.Content().Photos()); err != nil {
			return err
		}
	}

	return nil
}

func (s *ClubPurgeService) purgeTopics(ctx context.Context, clubID uuid.UUID) error {
	topics, messages, err := s.topics.DeleteByClub(ctx, clubID)
	if err != nil {
		return errs.Wrap("s.topics.DeleteByClub", err)
	}

	// The notifications about the messages name their topics.
	for _, id := range topics {
		if err = s.notifier.ForgetPost(ctx, id); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}
	}

	return errs.Wrap("deleteMessagePhotos", deleteMessagePhotos(ctx, s.media, messages))
}

// forgetAlbumPhotos deletes the images of photos, each under who uploaded
// it, and the notifications about them.
func (s *ClubPurgeService) forgetAlbumPhotos(ctx context.Context, photos []*post.AlbumPhoto) error {
	for _, p := range photos {
		if err := s.notifier.ForgetPost(ctx, p.ID()); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		if err := s.deletePhotos(ctx, p.AuthorID(), []uuid.UUID{p.ID()}); err != nil {
			return err
		}
	}

	return nil
}

// purgeWall deletes the posts on the wall with the comments on them.
func (s *ClubPurgeService) purgeWall(ctx context.Context, clubID uuid.UUID) error {
	comments, err := s.comments.DeleteOnPostsOf(ctx, clubID)
	if err != nil {
		return errs.Wrap("s.comments.DeleteOnPostsOf", err)
	}

	if err = deleteCommentPhotos(ctx, s.media, comments); err != nil {
		return errs.Wrap("deleteCommentPhotos", err)
	}

	deleted, err := s.posts.DeleteByUser(ctx, clubID)
	if err != nil {
		return errs.Wrap("s.posts.DeleteByUser", err)
	}

	for _, p := range deleted {
		if err = s.notifier.ForgetPost(ctx, p.ID()); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		if err = s.deletePhotos(ctx, p.AuthorID(), p.Content().OwnPhotos()); err != nil {
			return err
		}
	}

	return nil
}

func (s *ClubPurgeService) deletePhotos(ctx context.Context, ownerID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	return errs.Wrap("s.media.Delete", s.media.Delete(ctx, ownerID, ids))
}
