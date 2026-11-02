// Package inmem keeps federation in process memory, for development and
// tests: lost on restart.
package inmem

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

// TxManager runs fn as is: memory has no transactions.
type TxManager struct{}

func (TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type AccountRepository struct {
	mu       sync.Mutex
	accounts map[uuid.UUID]fedi.Account
}

func NewAccountRepository() *AccountRepository {
	return &AccountRepository{
		accounts: map[uuid.UUID]fedi.Account{},
	}
}

func (r *AccountRepository) Get(_ context.Context, userID uuid.UUID) (*fedi.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.accounts[userID]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return &a, nil
}

// Save keeps a new account, or whether a known one is open, as the
// database does: the keys of an account never change.
func (r *AccountRepository) Save(_ context.Context, a *fedi.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	saved, ok := r.accounts[a.UserID()]
	switch {
	case !ok:
		r.accounts[a.UserID()] = *a
	case a.Open():
		r.accounts[a.UserID()] = *saved.Opened()
	default:
		r.accounts[a.UserID()] = *saved.Closed()
	}

	return nil
}

type FollowerRepository struct {
	mu        sync.Mutex
	followers []fedi.Follower
}

func NewFollowerRepository() *FollowerRepository {
	return &FollowerRepository{}
}

func (r *FollowerRepository) Add(_ context.Context, f *fedi.Follower) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.followers = slices.DeleteFunc(r.followers, func(g fedi.Follower) bool {
		return g.UserID() == f.UserID() && g.Actor() == f.Actor()
	})
	r.followers = append(r.followers, *f)

	return nil
}

func (r *FollowerRepository) Remove(_ context.Context, userID uuid.UUID, actor fedi.URL) error {
	return r.removeWhere(func(f fedi.Follower) bool { return f.UserID() == userID && f.Actor() == actor })
}

func (r *FollowerRepository) RemoveActor(_ context.Context, actor fedi.URL) error {
	return r.removeWhere(func(f fedi.Follower) bool { return f.Actor() == actor })
}

func (r *FollowerRepository) RemoveAll(_ context.Context, userID uuid.UUID) error {
	return r.removeWhere(func(f fedi.Follower) bool { return f.UserID() == userID })
}

func (r *FollowerRepository) removeWhere(gone func(fedi.Follower) bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.followers = slices.DeleteFunc(r.followers, gone)

	return nil
}

func (r *FollowerRepository) Inboxes(_ context.Context, userID uuid.UUID) ([]fedi.URL, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var inboxes []fedi.URL
	for _, f := range r.followers {
		if f.UserID() == userID && !slices.Contains(inboxes, f.Inbox()) {
			inboxes = append(inboxes, f.Inbox())
		}
	}

	slices.Sort(inboxes)

	return inboxes, nil
}

func (r *FollowerRepository) Count(_ context.Context, userID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, f := range r.followers {
		if f.UserID() == userID {
			n++
		}
	}

	return n, nil
}

type RemoteRepository struct {
	mu      sync.Mutex
	remotes map[fedi.URL]fedi.Remote
}

func NewRemoteRepository() *RemoteRepository {
	return &RemoteRepository{
		remotes: map[fedi.URL]fedi.Remote{},
	}
}

func (r *RemoteRepository) Get(_ context.Context, id fedi.URL) (*fedi.Remote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.remotes[id]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return &a, nil
}

func (r *RemoteRepository) Save(_ context.Context, a *fedi.Remote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remotes[a.ID()] = *a

	return nil
}

func (r *RemoteRepository) Delete(_ context.Context, id fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.remotes, id)

	return nil
}
