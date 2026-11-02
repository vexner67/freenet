package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// PostChange is what happened to a post, as the post service tells.
type PostChange int

const (
	PostPublished PostChange = iota + 1
	PostEdited
	PostDeleted
)

// PostEvent is a change of a post of an author.
type PostEvent struct {
	Change   PostChange
	PostID   uuid.UUID
	AuthorID uuid.UUID
}

// PostChanged tells the followers of a page open to the Fediverse about
// its post: a new one, an edit, a deletion. A post that does not go to
// the Fediverse (see fedi.Post.Federates) or is gone already is left out.
// The post service tells at least once: the servers take an activity
// twice.
func (s *Service) PostChanged(ctx context.Context, e PostEvent) error {
	a, err := s.accounts.Get(ctx, e.AuthorID)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.accounts.Get", err)
	}

	if !a.Open() {
		return nil
	}

	inboxes, err := s.followers.Inboxes(ctx, e.AuthorID)
	if err != nil {
		return errs.Wrap("s.followers.Inboxes", err)
	}

	if len(inboxes) == 0 {
		return nil
	}

	activity, ok, err := s.postActivity(ctx, e)
	if err != nil || !ok {
		return err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		return s.sendAll(ctx, inboxes, e.AuthorID, activity)
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// postActivity is what to send of e; false when nothing.
func (s *Service) postActivity(ctx context.Context, e PostEvent) (fedi.Outgoing, bool, error) {
	actor := s.site.Actor(e.AuthorID)
	now := s.now()

	if e.Change == PostDeleted {
		return fedi.Outgoing{
			Type:      fedi.TypeDelete,
			ID:        s.site.Activity(e.PostID, fedi.TypeDelete, now),
			Actor:     actor,
			To:        []string{fedi.Public},
			Tombstone: s.site.Note(e.PostID),
		}, true, nil
	}

	p, err := s.posts.Post(ctx, e.PostID)
	if errors.Is(err, fedi.ErrNotFound) {
		return fedi.Outgoing{}, false, nil
	}
	if err != nil {
		return fedi.Outgoing{}, false, errs.Wrap("s.posts.Post", err)
	}

	if p.AuthorID != e.AuthorID || !p.Federates() {
		return fedi.Outgoing{}, false, nil
	}

	person, err := s.people.ByID(ctx, e.AuthorID)
	if err != nil {
		return fedi.Outgoing{}, false, errs.Wrap("s.people.ByID", err)
	}

	if person.Hidden {
		return fedi.Outgoing{}, false, nil
	}

	typ := fedi.TypeCreate
	if e.Change == PostEdited {
		typ = fedi.TypeUpdate
	}

	note := p.Note(s.site)

	return fedi.Outgoing{
		Type:  typ,
		ID:    s.site.Activity(e.PostID, typ, now),
		Actor: actor,
		To:    note.To,
		CC:    note.CC,
		Note:  &note,
	}, true, nil
}
