package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	// remoteMaxAge is how long the document of a remote actor is believed
	// before it is read again: keys and inboxes change.
	remoteMaxAge = 24 * time.Hour
	// inboxLimit is how many activities one server may send in a minute.
	inboxLimit  = 600
	inboxPeriod = time.Minute
)

// Deps are what the Service works with.
type Deps struct {
	Site       fedi.Site
	Accounts   AccountRepository
	Followers  FollowerRepository
	Remotes    RemoteRepository
	People     People
	Posts      Posts
	Resolver   Resolver
	Signatures Signatures
	Sender     Sender
	Keys       KeyMaker
	Limiter    Limiter
	Tx         TxManager
	Now        func() time.Time

	Following     FollowingRepository
	Subscriptions SubscriptionRepository
	News          RemotePostRepository
	Finder        Finder
	Media         MediaFetcher
	// Languages are those the news of everyone show posts in.
	Languages []string
	// Retention is how long remote posts are kept.
	Retention time.Duration
}

// Service opens pages to the Fediverse, tells other servers about them
// and their posts, and takes what those servers send.
type Service struct {
	site       fedi.Site
	accounts   AccountRepository
	followers  FollowerRepository
	remotes    RemoteRepository
	people     People
	posts      Posts
	resolver   Resolver
	signatures Signatures
	sender     Sender
	keys       KeyMaker
	limiter    Limiter
	tx         TxManager
	now        func() time.Time

	following     FollowingRepository
	subscriptions SubscriptionRepository
	news          RemotePostRepository
	finder        Finder
	media         MediaFetcher
	languages     []string
	retention     time.Duration
}

func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		site:       d.Site,
		accounts:   d.Accounts,
		followers:  d.Followers,
		remotes:    d.Remotes,
		people:     d.People,
		posts:      d.Posts,
		resolver:   d.Resolver,
		signatures: d.Signatures,
		sender:     d.Sender,
		keys:       d.Keys,
		limiter:    d.Limiter,
		tx:         d.Tx,
		now:        now,

		following:     d.Following,
		subscriptions: d.Subscriptions,
		news:          d.News,
		finder:        d.Finder,
		media:         d.Media,
		languages:     d.Languages,
		retention:     d.Retention,
	}
}

// Status is how a page stands with the Fediverse.
type Status struct {
	Open      bool
	Followers int
	// Domain ends the address people of other servers find the page by:
	// @nickname@domain.
	Domain string
}

func (s *Service) Status(ctx context.Context, userID uuid.UUID) (Status, error) {
	st := Status{Domain: s.site.Domain()}

	a, err := s.accounts.Get(ctx, userID)
	if errors.Is(err, fedi.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return Status{}, errs.Wrap("s.accounts.Get", err)
	}

	if st.Open = a.Open(); !st.Open {
		return st, nil
	}

	if st.Followers, err = s.followers.Count(ctx, userID); err != nil {
		return Status{}, errs.Wrap("s.followers.Count", err)
	}

	return st, nil
}

// Open opens the page to the Fediverse: its keys are made the first time.
// A page only some may see stays closed.
func (s *Service) Open(ctx context.Context, userID uuid.UUID) error {
	p, err := s.people.ByID(ctx, userID)
	if err != nil {
		return errs.Wrap("s.people.ByID", err)
	}

	if p.Hidden {
		return fedi.ErrPageHidden
	}

	a, err := s.accounts.Get(ctx, userID)
	switch {
	case errors.Is(err, fedi.ErrNotFound):
		keys, err := s.keys.NewKeys()
		if err != nil {
			return errs.Wrap("s.keys.NewKeys", err)
		}

		if a, err = fedi.NewAccount(userID, keys, s.now()); err != nil {
			return errs.Wrap("fedi.NewAccount", err)
		}
	case err != nil:
		return errs.Wrap("s.accounts.Get", err)
	case a.Open():
		return nil
	default:
		a = a.Opened()
	}

	return errs.Wrap("s.accounts.Save", s.accounts.Save(ctx, a))
}

// Close closes the page to the Fediverse: the servers of its followers
// hear that it is deleted, and it forgets them.
func (s *Service) Close(ctx context.Context, userID uuid.UUID) error {
	a, err := s.accounts.Get(ctx, userID)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.accounts.Get", err)
	}

	if !a.Open() {
		return nil
	}

	inboxes, err := s.followers.Inboxes(ctx, userID)
	if err != nil {
		return errs.Wrap("s.followers.Inboxes", err)
	}

	actor := s.site.Actor(userID)
	deletion := fedi.Outgoing{
		Type:   fedi.TypeDelete,
		ID:     s.site.Deletion(userID, s.now()),
		Actor:  actor,
		To:     []string{fedi.Public},
		Person: actor,
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.sendAll(ctx, inboxes, userID, deletion); err != nil {
			return errs.Wrap("s.sendAll", err)
		}

		if err := s.followers.RemoveAll(ctx, userID); err != nil {
			return errs.Wrap("s.followers.RemoveAll", err)
		}

		return errs.Wrap("s.accounts.Save", s.accounts.Save(ctx, a.Closed()))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

func (s *Service) sendAll(ctx context.Context, inboxes []fedi.URL, signerID uuid.UUID, a fedi.Outgoing) error {
	for _, inbox := range inboxes {
		if err := s.sender.Send(ctx, inbox, signerID, a); err != nil {
			return errs.Wrap("s.sender.Send", err)
		}
	}

	return nil
}

// open is the account of a page open to the Fediverse and the page as a
// guest sees it; any other is fedi.ErrNotFederated.
func (s *Service) open(ctx context.Context, userID uuid.UUID) (*fedi.Account, Person, error) {
	a, err := s.accounts.Get(ctx, userID)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil, Person{}, fedi.ErrNotFederated
	}
	if err != nil {
		return nil, Person{}, errs.Wrap("s.accounts.Get", err)
	}

	if !a.Open() {
		return nil, Person{}, fedi.ErrNotFederated
	}

	p, err := s.people.ByID(ctx, userID)
	if errors.Is(err, fedi.ErrNotFound) {
		return nil, Person{}, fedi.ErrNotFederated
	}
	if err != nil {
		return nil, Person{}, errs.Wrap("s.people.ByID", err)
	}

	if p.Hidden {
		return nil, Person{}, fedi.ErrNotFederated
	}

	return a, p, nil
}
