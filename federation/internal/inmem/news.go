package inmem

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
)

type FollowingRepository struct {
	mu        sync.Mutex
	following map[fedi.URL]fedi.Following
}

func NewFollowingRepository() *FollowingRepository {
	return &FollowingRepository{following: map[fedi.URL]fedi.Following{}}
}

func (r *FollowingRepository) Get(_ context.Context, actor fedi.URL) (*fedi.Following, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.following[actor]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return &f, nil
}

// Save keeps a new follow, or whether a known one is accepted and
// featured, as the database does.
func (r *FollowingRepository) Save(_ context.Context, f *fedi.Following) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if saved, ok := r.following[f.Actor()]; ok {
		f = saved.WithAccepted(f.Accepted()).WithFeatured(f.Featured())
	}
	r.following[f.Actor()] = *f

	return nil
}

func (r *FollowingRepository) Delete(_ context.Context, actor fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.following, actor)

	return nil
}

func (r *FollowingRepository) Featured(context.Context) ([]*fedi.Following, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []*fedi.Following
	for _, f := range r.following {
		if f.Featured() {
			out = append(out, &f)
		}
	}

	slices.SortFunc(out, func(a, b *fedi.Following) int { return a.CreatedAt().Compare(b.CreatedAt()) })

	return out, nil
}

func (r *FollowingRepository) featured(actor fedi.URL) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.following[actor]
	return ok && f.Featured()
}

type subscription struct {
	userID    uuid.UUID
	actor     fedi.URL
	createdAt time.Time
}

type SubscriptionRepository struct {
	mu   sync.Mutex
	subs []subscription
}

func NewSubscriptionRepository() *SubscriptionRepository {
	return &SubscriptionRepository{}
}

func (r *SubscriptionRepository) Add(_ context.Context, userID uuid.UUID, actor fedi.URL, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !slices.ContainsFunc(r.subs, func(s subscription) bool { return s.userID == userID && s.actor == actor }) {
		r.subs = append(r.subs, subscription{userID: userID, actor: actor, createdAt: now})
	}

	return nil
}

func (r *SubscriptionRepository) Remove(_ context.Context, userID uuid.UUID, actor fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subs = slices.DeleteFunc(r.subs, func(s subscription) bool { return s.userID == userID && s.actor == actor })

	return nil
}

func (r *SubscriptionRepository) RemoveActor(_ context.Context, actor fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subs = slices.DeleteFunc(r.subs, func(s subscription) bool { return s.actor == actor })

	return nil
}

func (r *SubscriptionRepository) Of(_ context.Context, userID uuid.UUID) ([]fedi.URL, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var mine []subscription
	for _, s := range r.subs {
		if s.userID == userID {
			mine = append(mine, s)
		}
	}

	slices.SortStableFunc(mine, func(a, b subscription) int { return b.createdAt.Compare(a.createdAt) })

	out := make([]fedi.URL, len(mine))
	for i, s := range mine {
		out[i] = s.actor
	}

	return out, nil
}

func (r *SubscriptionRepository) Read(_ context.Context, actor fedi.URL) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.ContainsFunc(r.subs, func(s subscription) bool { return s.actor == actor }), nil
}

func (r *SubscriptionRepository) reads(userID uuid.UUID, actor fedi.URL) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.ContainsFunc(r.subs, func(s subscription) bool { return s.userID == userID && s.actor == actor })
}

// RemotePostRepository reads the follows and subscriptions it lists the
// news by, as the database joins them.
type RemotePostRepository struct {
	mu            sync.Mutex
	posts         []fedi.RemotePost
	following     *FollowingRepository
	subscriptions *SubscriptionRepository
}

func NewRemotePostRepository(following *FollowingRepository, subscriptions *SubscriptionRepository) *RemotePostRepository {
	return &RemotePostRepository{following: following, subscriptions: subscriptions}
}

func (r *RemotePostRepository) find(match func(p fedi.RemotePost) bool) (*fedi.RemotePost, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	i := slices.IndexFunc(r.posts, match)
	if i < 0 {
		return nil, fedi.ErrNotFound
	}

	p := r.posts[i]
	return &p, nil
}

func (r *RemotePostRepository) Get(_ context.Context, id uuid.UUID) (*fedi.RemotePost, error) {
	return r.find(func(p fedi.RemotePost) bool { return p.ID() == id })
}

func (r *RemotePostRepository) ByURI(_ context.Context, uri fedi.URL) (*fedi.RemotePost, error) {
	return r.find(func(p fedi.RemotePost) bool { return p.URI() == uri })
}

// Save keeps a new note, or one edited under the ID it had.
func (r *RemotePostRepository) Save(_ context.Context, p *fedi.RemotePost) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if i := slices.IndexFunc(r.posts, func(q fedi.RemotePost) bool { return q.URI() == p.URI() }); i >= 0 {
		kept := r.posts[i]
		r.posts[i] = *fedi.RestoreRemotePost(kept.ID(), p.URI(), kept.Actor(), p.Page(), p.Text(), p.Summary(), p.Images(),
			p.Sensitive(), p.Language(), kept.Published(), p.Updated())
		return nil
	}

	r.posts = append(r.posts, *p)

	return nil
}

func (r *RemotePostRepository) Delete(_ context.Context, uri fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.posts = slices.DeleteFunc(r.posts, func(p fedi.RemotePost) bool { return p.URI() == uri })

	return nil
}

func (r *RemotePostRepository) DeleteActor(_ context.Context, actor fedi.URL) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.posts = slices.DeleteFunc(r.posts, func(p fedi.RemotePost) bool { return p.Actor() == actor })

	return nil
}

func (r *RemotePostRepository) List(_ context.Context, q service.NewsQuery) ([]*fedi.RemotePost, error) {
	r.mu.Lock()
	posts := slices.Clone(r.posts)
	r.mu.Unlock()

	slices.SortFunc(posts, func(a, b fedi.RemotePost) int { return cmp.Compare(b.ID().String(), a.ID().String()) })

	var out []*fedi.RemotePost
	for _, p := range posts {
		if len(out) == q.Limit {
			break
		}

		if q.Before != uuid.Nil && p.ID().String() >= q.Before.String() || q.HideSensitive && p.Sensitive() {
			continue
		}

		in := r.subscriptions.reads(q.Reader, p.Actor())
		if q.Featured {
			in = r.following.featured(p.Actor()) && p.Speaks(q.Languages)
		}

		if in {
			out = append(out, &p)
		}
	}

	return out, nil
}

func (r *RemotePostRepository) DeleteBefore(_ context.Context, t time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(r.posts)
	r.posts = slices.DeleteFunc(r.posts, func(p fedi.RemotePost) bool { return p.Published().Before(t) })

	return n - len(r.posts), nil
}
