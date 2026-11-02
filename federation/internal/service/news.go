package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// MaxSubscriptions is how many accounts of other servers one person may
// read: each costs the site a follow.
const MaxSubscriptions = 500

// A person looks up at most lookupLimit accounts of other servers in
// lookupPeriod.
const (
	lookupLimit  = 30
	lookupPeriod = time.Minute
)

// Account is an account of another server as the site shows it.
type Account struct {
	Remote *fedi.Remote
	// Accepted: its server took the follow of the site; until then its
	// posts may not come.
	Accepted bool
}

// NewsPost is a remote post with its author.
type NewsPost struct {
	Post   *fedi.RemotePost
	Author *fedi.Remote
}

// site is the account of the site itself, made the first time it is
// needed. A replica making it at the same time keeps the first one made:
// the keys of an account never change.
func (s *Service) siteAccount(ctx context.Context) (*fedi.Account, error) {
	a, err := s.accounts.Get(ctx, uuid.Nil)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, fedi.ErrNotFound) {
		return nil, errs.Wrap("s.accounts.Get", err)
	}

	keys, err := s.keys.NewKeys()
	if err != nil {
		return nil, errs.Wrap("s.keys.NewKeys", err)
	}

	if a, err = fedi.NewAccount(uuid.Nil, keys, s.now()); err != nil {
		return nil, errs.Wrap("fedi.NewAccount", err)
	}

	if err = s.accounts.Save(ctx, a); err != nil {
		return nil, errs.Wrap("s.accounts.Save", err)
	}

	a, err = s.accounts.Get(ctx, uuid.Nil)
	return a, errs.Wrap("s.accounts.Get", err)
}

// SiteActor is the document of the site's own actor.
func (s *Service) SiteActor(ctx context.Context) (fedi.Person, error) {
	a, err := s.siteAccount(ctx)
	if err != nil {
		return fedi.Person{}, err
	}

	return fedi.Person{
		ID:        s.site.Actor(uuid.Nil),
		Page:      s.site.ProfilePage(""),
		Nickname:  s.site.Domain(),
		Name:      "Freenet",
		Inbox:     s.site.Inbox(uuid.Nil),
		Outbox:    s.site.Outbox(uuid.Nil),
		Followers: s.site.Followers(uuid.Nil),
		Shared:    s.site.SharedInbox(),
		KeyID:     s.site.Key(uuid.Nil),
		PublicKey: a.Keys().Public,
		Published: a.CreatedAt(),
		Service:   true,
	}, nil
}

// Lookup is the account of another server an address names.
func (s *Service) Lookup(ctx context.Context, address string) (*fedi.Remote, error) {
	addr, err := fedi.NewAddress(address, s.site)
	if err != nil {
		return nil, err
	}

	signer, err := s.siteAccount(ctx)
	if err != nil {
		return nil, err
	}

	id := addr.URL
	if id == "" {
		if id, err = s.finder.Finger(ctx, addr.Username, addr.Host); err != nil {
			return nil, unreachable(ctx, errs.Wrap("s.finder.Finger", err))
		}
	}

	r, _, err := s.remote(ctx, id, signer, false)
	if err != nil {
		return nil, unreachable(ctx, err)
	}

	return r, nil
}

// unreachable is why an account of another server could not be read: to
// the person looking, there is no such account, whatever its server did.
func unreachable(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return err
	}

	return fmt.Errorf("%w: %w", fedi.ErrAccountNotFound, err)
}

// Find is the account of another server an address names, as userID
// sees it: whether the site follows it and userID reads it.
func (s *Service) Find(ctx context.Context, userID uuid.UUID, address string) (Account, bool, error) {
	if err := s.allowLookup(ctx, userID); err != nil {
		return Account{}, false, err
	}

	r, err := s.Lookup(ctx, address)
	if err != nil {
		return Account{}, false, err
	}

	a, err := s.account(ctx, r)
	if err != nil {
		return Account{}, false, err
	}

	mine, err := s.subscriptions.Of(ctx, userID)
	if err != nil {
		return Account{}, false, errs.Wrap("s.subscriptions.Of", err)
	}

	return a, slices.Contains(mine, r.ID()), nil
}

func (s *Service) allowLookup(ctx context.Context, userID uuid.UUID) error {
	ok, err := s.limiter.Allow(ctx, "lookup:"+userID.String(), lookupLimit, lookupPeriod)
	if err != nil {
		return errs.Wrap("s.limiter.Allow", err)
	}

	if !ok {
		return fedi.ErrTooManyLookups
	}

	return nil
}

// account is r with whether its server took the follow of the site.
func (s *Service) account(ctx context.Context, r *fedi.Remote) (Account, error) {
	f, err := s.following.Get(ctx, r.ID())
	if err != nil && !errors.Is(err, fedi.ErrNotFound) {
		return Account{}, errs.Wrap("s.following.Get", err)
	}

	return Account{Remote: r, Accepted: f != nil && f.Accepted()}, nil
}

// Feature puts an account of another server in the news of everyone.
func (s *Service) Feature(ctx context.Context, address string) (Account, error) {
	r, err := s.Lookup(ctx, address)
	if err != nil {
		return Account{}, err
	}

	if err = s.followRemote(ctx, r, true); err != nil {
		return Account{}, err
	}

	return s.account(ctx, r)
}

// Unfeature takes an account out of the news of everyone; the site stops
// following it unless somebody here reads it.
func (s *Service) Unfeature(ctx context.Context, actor fedi.URL) error {
	f, err := s.following.Get(ctx, actor)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.following.Get", err)
	}

	if err = s.following.Save(ctx, f.WithFeatured(false)); err != nil {
		return errs.Wrap("s.following.Save", err)
	}

	return s.unfollowIfUnread(ctx, actor)
}

// Featured are the accounts in the news of everyone.
func (s *Service) Featured(ctx context.Context) ([]Account, error) {
	fs, err := s.following.Featured(ctx)
	if err != nil {
		return nil, errs.Wrap("s.following.Featured", err)
	}

	out := make([]Account, 0, len(fs))
	for _, f := range fs {
		r, err := s.known(ctx, f.Actor())
		if err != nil {
			return nil, err
		}
		out = append(out, Account{Remote: r, Accepted: f.Accepted()})
	}

	return out, nil
}

// Subscribe puts an account of another server in the news of userID.
func (s *Service) Subscribe(ctx context.Context, userID uuid.UUID, address string) (Account, error) {
	mine, err := s.subscriptions.Of(ctx, userID)
	if err != nil {
		return Account{}, errs.Wrap("s.subscriptions.Of", err)
	}

	if len(mine) >= MaxSubscriptions {
		return Account{}, fedi.ErrTooManySubscriptions
	}

	if err = s.allowLookup(ctx, userID); err != nil {
		return Account{}, err
	}

	r, err := s.Lookup(ctx, address)
	if err != nil {
		return Account{}, err
	}

	if err = s.subscriptions.Add(ctx, userID, r.ID(), s.now()); err != nil {
		return Account{}, errs.Wrap("s.subscriptions.Add", err)
	}

	if err = s.followRemote(ctx, r, false); err != nil {
		return Account{}, err
	}

	return s.account(ctx, r)
}

// Unsubscribe takes an account out of the news of userID.
func (s *Service) Unsubscribe(ctx context.Context, userID uuid.UUID, actor fedi.URL) error {
	if err := s.subscriptions.Remove(ctx, userID, actor); err != nil {
		return errs.Wrap("s.subscriptions.Remove", err)
	}

	return s.unfollowIfUnread(ctx, actor)
}

// Subscriptions are the accounts of other servers userID reads.
func (s *Service) Subscriptions(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	actors, err := s.subscriptions.Of(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.subscriptions.Of", err)
	}

	out := make([]Account, 0, len(actors))
	for _, actor := range actors {
		r, err := s.known(ctx, actor)
		if err != nil {
			return nil, err
		}

		a, err := s.account(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}

	return out, nil
}

// followRemote makes the site follow r, once: featured or not is kept as asked
// when it already does, but never taken back here.
func (s *Service) followRemote(ctx context.Context, r *fedi.Remote, featured bool) error {
	f, err := s.following.Get(ctx, r.ID())
	switch {
	case err == nil:
		if featured && !f.Featured() {
			return errs.Wrap("s.following.Save", s.following.Save(ctx, f.WithFeatured(true)))
		}
		return nil
	case !errors.Is(err, fedi.ErrNotFound):
		return errs.Wrap("s.following.Get", err)
	}

	if f, err = fedi.NewFollowing(r.ID(), featured, s.now()); err != nil {
		return errs.Wrap("fedi.NewFollowing", err)
	}

	follow := fedi.Outgoing{
		Type:   fedi.TypeFollow,
		ID:     s.site.Follow(f.FollowID()),
		Actor:  s.site.Actor(uuid.Nil),
		To:     []string{r.ID().String()},
		Person: r.ID(),
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.following.Save(ctx, f); err != nil {
			return errs.Wrap("s.following.Save", err)
		}

		return errs.Wrap("s.sender.Send", s.sender.Send(ctx, r.Inbox(), uuid.Nil, follow))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// unfollowIfUnread stops following an account neither featured nor read
// by anybody here, and forgets its posts.
func (s *Service) unfollowIfUnread(ctx context.Context, actor fedi.URL) error {
	f, err := s.following.Get(ctx, actor)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.following.Get", err)
	}

	read, err := s.subscriptions.Read(ctx, actor)
	if err != nil {
		return errs.Wrap("s.subscriptions.Read", err)
	}

	if f.Featured() || read {
		return nil
	}

	r, err := s.known(ctx, actor)
	if err != nil {
		return err
	}

	undo := fedi.Outgoing{
		Type:  fedi.TypeUndo,
		ID:    s.site.Unfollow(f.FollowID()),
		Actor: s.site.Actor(uuid.Nil),
		To:    []string{actor.String()},
		Follow: &fedi.Incoming{
			Type: fedi.TypeFollow, ID: s.site.Follow(f.FollowID()).String(), Actor: s.site.Actor(uuid.Nil), Object: actor.String(),
		},
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.following.Delete(ctx, actor); err != nil {
			return errs.Wrap("s.following.Delete", err)
		}

		if err := s.news.DeleteActor(ctx, actor); err != nil {
			return errs.Wrap("s.news.DeleteActor", err)
		}

		if r.Inbox() == "" {
			return nil
		}

		return errs.Wrap("s.sender.Send", s.sender.Send(ctx, r.Inbox(), uuid.Nil, undo))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// known is the document of an actor the site follows: the one kept, as
// old as it may be, or one with its address alone.
func (s *Service) known(ctx context.Context, actor fedi.URL) (*fedi.Remote, error) {
	r, err := s.remotes.Get(ctx, actor)
	if errors.Is(err, fedi.ErrNotFound) {
		return fedi.RestoreRemote(actor, "", "", "", "", fedi.Profile{}, s.now()), nil
	}

	return r, errs.Wrap("s.remotes.Get", err)
}

// News are remote posts for the news, newest first.
func (s *Service) News(ctx context.Context, q NewsQuery) ([]NewsPost, error) {
	if q.Featured {
		q.Languages = s.languages
	}

	posts, err := s.news.List(ctx, q)
	if err != nil {
		return nil, errs.Wrap("s.news.List", err)
	}

	authors := map[fedi.URL]*fedi.Remote{}
	out := make([]NewsPost, 0, len(posts))

	for _, p := range posts {
		author, ok := authors[p.Actor()]
		if !ok {
			if author, err = s.known(ctx, p.Actor()); err != nil {
				return nil, err
			}
			authors[p.Actor()] = author
		}

		out = append(out, NewsPost{Post: p, Author: author})
	}

	return out, nil
}

// Picture is the image index of a remote post, read from its server.
func (s *Service) Picture(ctx context.Context, postID uuid.UUID, index int) (Media, error) {
	p, err := s.news.Get(ctx, postID)
	if err != nil {
		return Media{}, errs.Wrap("s.news.Get", err)
	}

	images := p.Images()
	if index < 0 || index >= len(images) {
		return Media{}, fedi.ErrNotFound
	}

	m, err := s.media.Fetch(ctx, images[index].URL)
	return m, errs.Wrap("s.media.Fetch", err)
}

// VideoCover is the cover of the video index of a remote post: the one
// its author chose, or else its first frame, cut once and kept.
func (s *Service) VideoCover(ctx context.Context, postID uuid.UUID, index int) (Media, error) {
	p, err := s.news.Get(ctx, postID)
	if err != nil {
		return Media{}, errs.Wrap("s.news.Get", err)
	}

	videos := p.Videos()
	if index < 0 || index >= len(videos) {
		return Media{}, fedi.ErrNotFound
	}

	v := videos[index]
	if v.Cover != "" {
		m, err := s.media.Fetch(ctx, v.Cover)
		return m, errs.Wrap("s.media.Fetch", err)
	}

	frame, err := s.covers.Get(ctx, postID, v.URL)
	switch {
	case errors.Is(err, fedi.ErrNotFound):
		if frame, err = s.cut(ctx, postID, v.URL); err != nil {
			return Media{}, err
		}
	case err != nil:
		return Media{}, errs.Wrap("s.covers.Get", err)
	}

	if len(frame) == 0 {
		return Media{}, fedi.ErrNotFound
	}

	return Media{Body: frame, ContentType: "image/jpeg"}, nil
}

// cut cuts the first frame of a video and keeps it, or that it has none,
// so that it is cut once.
func (s *Service) cut(ctx context.Context, postID uuid.UUID, video fedi.URL) ([]byte, error) {
	frame, err := s.frames.Frame(ctx, video)
	if err != nil && !errors.Is(err, fedi.ErrNotFound) {
		return nil, errs.Wrap("s.frames.Frame", err)
	}

	if err = s.covers.Save(ctx, postID, video, frame); err != nil {
		return nil, errs.Wrap("s.covers.Save", err)
	}

	return frame, nil
}

// Avatar is the photo of an account of another server the site knows,
// read from its server: only those, so that the site is no proxy of any
// address.
func (s *Service) Avatar(ctx context.Context, actor fedi.URL) (Media, error) {
	r, err := s.remotes.Get(ctx, actor)
	if err != nil {
		return Media{}, errs.Wrap("s.remotes.Get", err)
	}

	if r.Profile().Icon == "" {
		return Media{}, fedi.ErrNotFound
	}

	m, err := s.media.Fetch(ctx, r.Profile().Icon)
	return m, errs.Wrap("s.media.Fetch", err)
}

// Cleanup forgets the remote posts older than the news keep them.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	n, err := s.news.DeleteBefore(ctx, s.now().Add(-s.retention))
	return n, errs.Wrap("s.news.DeleteBefore", err)
}
