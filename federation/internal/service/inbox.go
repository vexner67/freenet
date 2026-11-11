package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// Receive takes an activity another server sent to an inbox here: the
// one of the page to (uuid.Nil for the shared inbox). Only what the
// service acts on is checked, as a check costs a request to the sender's
// server: following and unfollowing a page, an actor deleted, a follow of
// the site accepted or rejected, and the notes of the accounts the site
// follows. Anything else is taken and dropped.
func (s *Service) Receive(ctx context.Context, r SignedRequest, a fedi.Incoming, to uuid.UUID) error {
	ok, err := s.limiter.Allow(ctx, "inbox:"+a.Actor.Host(), inboxLimit, inboxPeriod)
	if err != nil {
		return errs.Wrap("s.limiter.Allow", err)
	}

	if !ok {
		return fedi.ErrTooMany
	}

	switch {
	case a.Type == fedi.TypeFollow:
		return s.follow(ctx, r, a)
	case a.Type == fedi.TypeUndo && a.Inner != nil && a.Inner.Type == fedi.TypeFollow:
		return s.unfollow(ctx, r, a)
	case a.DeletesActor():
		return s.forget(ctx, r, a, to)
	case a.Type == fedi.TypeAccept || a.Type == fedi.TypeReject:
		return s.answered(ctx, r, a)
	case a.Type == fedi.TypeCreate && a.Note != nil:
		return s.noted(ctx, r, a)
	case a.Type == fedi.TypeUpdate && a.Note != nil:
		return s.edited(ctx, r, a)
	case a.Type == fedi.TypeDelete:
		return s.unnoted(ctx, r, a)
	default:
		return nil
	}
}

// answered is the answer of a server to a follow of the site.
func (s *Service) answered(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	f, err := s.following.Get(ctx, a.Actor)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.following.Get", err)
	}

	if fedi.URL(a.Object) != s.site.Follow(f.FollowID()) {
		return nil
	}

	if _, err = s.verifiedBySite(ctx, r, a); err != nil {
		return err
	}

	return errs.Wrap("s.following.Save", s.following.Save(ctx, f.WithAccepted(a.Type == fedi.TypeAccept)))
}

// noted keeps a new note of an account the site follows.
func (s *Service) noted(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	if _, err := s.following.Get(ctx, a.Actor); err != nil {
		if errors.Is(err, fedi.ErrNotFound) {
			return nil
		}
		return errs.Wrap("s.following.Get", err)
	}

	p, err := fedi.NewRemotePost(*a.Note, a.Actor, s.now())
	if errors.Is(err, fedi.ErrNotForNews) {
		return nil
	}
	if err != nil {
		return errs.Wrap("fedi.NewRemotePost", err)
	}

	// A note sent again is taken as it is kept.
	if _, err = s.news.ByURI(ctx, p.URI()); err == nil {
		return nil
	}
	if !errors.Is(err, fedi.ErrNotFound) {
		return errs.Wrap("s.news.ByURI", err)
	}

	if _, err = s.verifiedBySite(ctx, r, a); err != nil {
		return err
	}

	return errs.Wrap("s.news.Save", s.news.Save(ctx, p))
}

// edited changes a note kept: an edit that makes it no note for the news
// forgets it.
func (s *Service) edited(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	kept, err := s.news.ByURI(ctx, fedi.URL(a.Note.ID))
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.news.ByURI", err)
	}

	if kept.Actor() != a.Actor {
		return nil
	}

	if _, err = s.verifiedBySite(ctx, r, a); err != nil {
		return err
	}

	p, err := kept.Edited(*a.Note, s.now())
	if errors.Is(err, fedi.ErrNotForNews) {
		return errs.Wrap("s.news.Delete", s.news.Delete(ctx, kept.URI()))
	}
	if err != nil {
		return errs.Wrap("kept.Edited", err)
	}

	return errs.Wrap("s.news.Save", s.news.Save(ctx, p))
}

// unnoted forgets a note deleted on its server.
func (s *Service) unnoted(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	kept, err := s.news.ByURI(ctx, fedi.URL(a.Object))
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.news.ByURI", err)
	}

	if kept.Actor() != a.Actor {
		return nil
	}

	if _, err = s.verifiedBySite(ctx, r, a); err != nil {
		return err
	}

	return errs.Wrap("s.news.Delete", s.news.Delete(ctx, kept.URI()))
}

// verifiedBySite is verified with the reads of the actor signed by the
// site, as servers that want signed reads require.
func (s *Service) verifiedBySite(ctx context.Context, r SignedRequest, a fedi.Incoming) (*fedi.Remote, error) {
	signer, err := s.siteAccount(ctx)
	if err != nil {
		return nil, err
	}

	return s.verified(ctx, r, a, signer)
}

func (s *Service) follow(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	userID, ok := s.site.UserOf(fedi.URL(a.Object))
	if !ok {
		return fedi.ErrNotFound
	}

	account, _, err := s.open(ctx, userID)
	if err != nil {
		return err
	}

	remote, err := s.verified(ctx, r, a, account)
	if err != nil {
		return err
	}

	accept := fedi.Outgoing{
		Type:   fedi.TypeAccept,
		ID:     s.site.Accept(userID, newID()),
		Actor:  s.site.Actor(userID),
		To:     []string{remote.ID().String()},
		Follow: &a,
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.followers.Add(ctx, fedi.NewFollower(userID, remote, s.now())); err != nil {
			return errs.Wrap("s.followers.Add", err)
		}

		return errs.Wrap("s.sender.Send", s.sender.Send(ctx, remote.Inbox(), userID, accept))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

func (s *Service) unfollow(ctx context.Context, r SignedRequest, a fedi.Incoming) error {
	userID, ok := s.site.UserOf(fedi.URL(a.Inner.Object))
	if !ok || a.Inner.Actor != a.Actor {
		return nil
	}

	account, err := s.accounts.Get(ctx, userID)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.accounts.Get", err)
	}

	if _, err = s.verified(ctx, r, a, account); err != nil {
		return err
	}

	return errs.Wrap("s.followers.Remove", s.followers.Remove(ctx, userID, a.Actor))
}

// forget drops an actor deleted on its server. Its key may be gone with
// it: a server that says the actor is gone for good is believed instead.
func (s *Service) forget(ctx context.Context, r SignedRequest, a fedi.Incoming, to uuid.UUID) error {
	var signer *fedi.Account

	if to != uuid.Nil {
		account, err := s.accounts.Get(ctx, to)
		if err != nil && !errors.Is(err, fedi.ErrNotFound) {
			return errs.Wrap("s.accounts.Get", err)
		}
		signer = account
	}

	_, err := s.verified(ctx, r, a, signer)
	if err != nil && !errors.Is(err, fedi.ErrGone) {
		return err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.followers.RemoveActor(ctx, a.Actor); err != nil {
			return errs.Wrap("s.followers.RemoveActor", err)
		}

		if err := s.subscriptions.RemoveActor(ctx, a.Actor); err != nil {
			return errs.Wrap("s.subscriptions.RemoveActor", err)
		}

		if err := s.following.Delete(ctx, a.Actor); err != nil {
			return errs.Wrap("s.following.Delete", err)
		}

		if err := s.news.DeleteActor(ctx, a.Actor); err != nil {
			return errs.Wrap("s.news.DeleteActor", err)
		}

		return errs.Wrap("s.remotes.Delete", s.remotes.Delete(ctx, a.Actor))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// verified is the actor of a, once the request is found signed with its
// key. A key that fails is read again once from the actor's server: it
// may have changed it.
func (s *Service) verified(ctx context.Context, r SignedRequest, a fedi.Incoming, signer *fedi.Account) (*fedi.Remote, error) {
	keyID, err := s.signatures.KeyID(r)
	if err != nil {
		return nil, errs.Wrap("s.signatures.KeyID", err)
	}

	remote, fresh, err := s.remote(ctx, a.Actor, signer, false)
	if err != nil {
		return nil, err
	}

	if s.signedBy(r, keyID, remote) {
		return remote, nil
	}

	if fresh {
		return nil, fedi.ErrBadSignature
	}

	if remote, _, err = s.remote(ctx, a.Actor, signer, true); err != nil {
		return nil, err
	}

	if !s.signedBy(r, keyID, remote) {
		return nil, fedi.ErrBadSignature
	}

	return remote, nil
}

func (s *Service) signedBy(r SignedRequest, keyID fedi.URL, remote *fedi.Remote) bool {
	return keyID == remote.KeyID() && s.signatures.Verify(r, remote.PublicKey(), s.now()) == nil
}

// remote is the document of an actor: the one read lately, unless reload,
// or read now (fresh).
func (s *Service) remote(ctx context.Context, id fedi.URL, signer *fedi.Account, reload bool) (*fedi.Remote, bool, error) {
	if !reload {
		r, err := s.remotes.Get(ctx, id)
		if err == nil && !r.Stale(s.now(), remoteMaxAge) {
			return r, false, nil
		}
		if err != nil && !errors.Is(err, fedi.ErrNotFound) {
			return nil, false, errs.Wrap("s.remotes.Get", err)
		}
	}

	// A shared inbox has no recipient key. Some servers require a signed
	// fetch even for the actor of a Delete, so use the site's key.
	if signer == nil {
		account, err := s.siteAccount(ctx)
		if err != nil {
			return nil, false, errs.Wrap("s.siteAccount", err)
		}

		signer = account
	}

	doc, err := s.resolver.Actor(ctx, id, signer)
	if err != nil {
		return nil, false, errs.Wrap("s.resolver.Actor", err)
	}

	r, err := fedi.NewRemote(id, doc, s.now())
	if err != nil {
		return nil, false, errs.Wrap("fedi.NewRemote", err)
	}

	if err = s.remotes.Save(ctx, r); err != nil {
		return nil, false, errs.Wrap("s.remotes.Save", err)
	}

	return r, true, nil
}

// newID is a new UUIDv7, or a random UUID should the clock fail it.
func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}

	return id
}
