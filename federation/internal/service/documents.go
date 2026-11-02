package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// Finger is the page a WebFinger resource names (acct:nick@domain or the
// address of its page), when it is open to the Fediverse.
func (s *Service) Finger(ctx context.Context, resource string) (fedi.Person, error) {
	nickname, ok := s.site.Nickname(resource)
	if !ok {
		return fedi.Person{}, fedi.ErrNotFound
	}

	p, err := s.people.ByNickname(ctx, nickname)
	if errors.Is(err, fedi.ErrNotFound) {
		return fedi.Person{}, fedi.ErrNotFederated
	}
	if err != nil {
		return fedi.Person{}, errs.Wrap("s.people.ByNickname", err)
	}

	return s.Actor(ctx, p.ID)
}

// Actor is the document of a page open to the Fediverse.
func (s *Service) Actor(ctx context.Context, userID uuid.UUID) (fedi.Person, error) {
	a, p, err := s.open(ctx, userID)
	if err != nil {
		return fedi.Person{}, err
	}

	person := fedi.Person{
		ID:        s.site.Actor(userID),
		Page:      s.site.ProfilePage(p.Nickname),
		Nickname:  p.Nickname,
		Name:      p.Name,
		Summary:   fedi.TextHTML(p.Status, s.site),
		Inbox:     s.site.Inbox(userID),
		Outbox:    s.site.Outbox(userID),
		Followers: s.site.Followers(userID),
		Shared:    s.site.SharedInbox(),
		KeyID:     s.site.Key(userID),
		PublicKey: a.Keys().Public,
		Published: p.RegisteredAt,
	}

	if p.AvatarID != "" {
		person.Avatar = s.site.Avatar(userID, p.AvatarID)
	}

	return person, nil
}

// Note is a post of a page open to the Fediverse, if it goes there.
func (s *Service) Note(ctx context.Context, postID uuid.UUID) (fedi.Note, error) {
	p, err := s.posts.Post(ctx, postID)
	if err != nil {
		return fedi.Note{}, errs.Wrap("s.posts.Post", err)
	}

	if !p.Federates() {
		return fedi.Note{}, fedi.ErrNotFound
	}

	if _, _, err = s.open(ctx, p.AuthorID); err != nil {
		return fedi.Note{}, err
	}

	return p.Note(s.site), nil
}

// FollowerCount is how many people of other servers follow a page open
// to the Fediverse; who they are is not told.
func (s *Service) FollowerCount(ctx context.Context, userID uuid.UUID) (int, error) {
	if _, _, err := s.open(ctx, userID); err != nil {
		return 0, err
	}

	n, err := s.followers.Count(ctx, userID)
	if err != nil {
		return 0, errs.Wrap("s.followers.Count", err)
	}

	return n, nil
}
