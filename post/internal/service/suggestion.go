package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type SuggestionRepository interface {
	Save(ctx context.Context, s *post.Suggestion) error
	// Get and Delete return post.ErrSuggestionNotFound; Delete does when
	// the suggestion was published or withdrawn meanwhile.
	Get(ctx context.Context, id uuid.UUID) (*post.Suggestion, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit suggestions for a wall, only those of
	// authorID unless it is uuid.Nil, newest first, with IDs below before
	// (uuid.Nil: from the newest); Count counts them all.
	List(ctx context.Context, wallID, authorID, before uuid.UUID, limit int) ([]*post.Suggestion, error)
	Count(ctx context.Context, wallID, authorID uuid.UUID) (int, error)
}

// SuggestionModerators are the WallModerators that also list the admins
// of a community, who hear about the posts suggested for its wall.
type SuggestionModerators interface {
	WallModerators
	// Moderators are the admins of the community wallID, none for the
	// wall of a user.
	Moderators(ctx context.Context, wallID uuid.UUID) ([]uuid.UUID, error)
}

// SuggestionPage is one screen of suggestions; NextCursor is uuid.Nil on
// the last, Total counts them on all pages.
type SuggestionPage struct {
	Suggestions []*post.Suggestion
	NextCursor  uuid.UUID
	Total       int
}

// SuggestionService keeps the posts suggested for communities' walls
// ("Предложка" of openvk) until their admins publish or decline them.
type SuggestionService struct {
	suggestions SuggestionRepository
	posts       PostRepository
	tx          TxManager
	notifier    Notifier
	photos      PhotoKeeper
	moderators  SuggestionModerators
	now         func() time.Time
}

// SuggestionDeps are what SuggestionService works with.
type SuggestionDeps struct {
	Suggestions SuggestionRepository
	Posts       PostRepository
	Tx          TxManager
	Notifier    Notifier
	Photos      PhotoKeeper
	Moderators  SuggestionModerators
}

func NewSuggestionService(d SuggestionDeps) *SuggestionService {
	return &SuggestionService{
		suggestions: d.Suggestions,
		posts:       d.Posts,
		tx:          d.Tx,
		notifier:    d.Notifier,
		photos:      d.Photos,
		moderators:  d.Moderators,
		now:         time.Now,
	}
}

// Suggest keeps content by authorID for the admins of the community
// wallID to publish, and tells them from the community ("новые записи в
// предложке" of openvk) until it is published, declined or withdrawn. Its
// photos are kept for good at once, as a post's: a suggestion may wait
// longer than media keeps unpublished photos. The caller has checked that
// the author may suggest there and that the photos are theirs.
func (s *SuggestionService) Suggest(ctx context.Context, wallID, authorID uuid.UUID, content post.Content) (*post.Suggestion, error) {
	suggestion, err := post.NewSuggestion(wallID, authorID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewSuggestion", err)
	}

	admins, err := s.moderators.Moderators(ctx, wallID)
	if err != nil {
		return nil, errs.Wrap("s.moderators.Moderators", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.suggestions.Save(ctx, suggestion); err != nil {
			return errs.Wrap("s.suggestions.Save", err)
		}

		for _, admin := range admins {
			if admin == authorID {
				continue
			}

			e := Event{Kind: EventSuggestedPost, RecipientID: admin, ActorID: wallID, PostID: suggestion.ID()}
			if err := s.notifier.Notify(ctx, e); err != nil {
				return errs.Wrap("s.notifier.Notify", err)
			}
		}

		if photos := content.Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Publish", s.photos.Publish(ctx, authorID, photos))
		}

		return nil
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return suggestion, nil
}

// List pages the suggestions for a wall, newest first: with mine, those
// viewerID suggested ("Предложенные вами записи"); otherwise all of them,
// which only the community's admins see ("Предложенные записи").
func (s *SuggestionService) List(ctx context.Context, viewerID, wallID uuid.UUID, mine bool, cursor uuid.UUID, size int) (SuggestionPage, error) {
	authorID := viewerID
	if !mine {
		ok, err := s.moderators.CanModerate(ctx, wallID, viewerID)
		if err != nil {
			return SuggestionPage{}, errs.Wrap("s.moderators.CanModerate", err)
		}

		if !ok {
			return SuggestionPage{}, post.ErrCannotSeeAll
		}

		authorID = uuid.Nil
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.suggestions.List(ctx, wallID, authorID, cursor, size+1)
	if err != nil {
		return SuggestionPage{}, errs.Wrap("s.suggestions.List", err)
	}

	page := SuggestionPage{Suggestions: found}
	if len(found) > size {
		page.Suggestions = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.suggestions.Count(ctx, wallID, authorID); err != nil {
		return SuggestionPage{}, errs.Wrap("s.suggestions.Count", err)
	}

	return page, nil
}

// Publish makes a post of a suggestion, an admin's "Опубликовать запись":
// it speaks for the community, says text and, signed, names the author
// (see post.Suggestion.Published). The author hears about it from the
// community, unless they published it themselves; the admins' notifications
// about the suggestion go.
func (s *SuggestionService) Publish(ctx context.Context, requesterID, id uuid.UUID, text post.Text, signed bool) (*post.Post, error) {
	suggestion, err := s.suggestions.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.suggestions.Get", err)
	}

	ok, err := s.moderators.CanModerate(ctx, suggestion.WallID(), requesterID)
	if err != nil {
		return nil, errs.Wrap("s.moderators.CanModerate", err)
	}

	if !ok {
		return nil, post.ErrCannotPublish
	}

	published, err := suggestion.Published(text, signed, s.now())
	if err != nil {
		return nil, errs.Wrap("suggestion.Published", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Published or withdrawn meanwhile: the post is not made twice.
		if err := s.suggestions.Delete(ctx, id); err != nil {
			return errs.Wrap("s.suggestions.Delete", err)
		}

		// The admins no longer need to hear about it.
		if err := s.notifier.ForgetPost(ctx, id); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		// The photos were kept when the post was suggested.
		if err := s.posts.Save(ctx, published); err != nil {
			return errs.Wrap("s.posts.Save", err)
		}

		if suggestion.AuthorID() == requesterID {
			return nil
		}

		e := Event{Kind: EventPostAccepted, RecipientID: suggestion.AuthorID(), ActorID: suggestion.WallID(), PostID: published.ID()}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, e))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return published, nil
}

// Withdraw deletes a suggestion with its photos: its author takes it
// back, or an admin of the community declines it ("Отклонить"); the
// admins' notifications about it go too.
func (s *SuggestionService) Withdraw(ctx context.Context, requesterID, id uuid.UUID) error {
	suggestion, err := s.suggestions.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.suggestions.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, suggestion.WallID(), requesterID, suggestion.CanBeWithdrawnBy(requesterID))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotWithdraw
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.suggestions.Delete(ctx, id); err != nil {
			return errs.Wrap("s.suggestions.Delete", err)
		}

		if err := s.notifier.ForgetPost(ctx, id); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		if photos := suggestion.Content().Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, suggestion.AuthorID(), photos))
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
