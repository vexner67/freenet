package inmem

import (
	"cmp"
	"context"
	"errors"
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

// Channels are the channels of Telegram followed, in the order of their
// addresses, as the database gives them.
func (r *FollowingRepository) Channels(context.Context) ([]fedi.URL, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []fedi.URL
	for actor := range r.following {
		if fedi.IsChannel(actor) {
			out = append(out, actor)
		}
	}

	slices.Sort(out)

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
		r.posts[i] = *fedi.RestoreRemotePost(kept.ID(), p.URI(), kept.Actor(), p.Page(), p.Text(), p.Summary(), p.Images(), p.Videos(),
			p.Sensitive(), p.Language(), kept.Published(), p.Updated()).WithLink(p.Link())
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

func (r *RemotePostRepository) URIsOf(_ context.Context, actor fedi.URL) ([]fedi.URL, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []fedi.URL
	for _, p := range r.posts {
		if p.Actor() == actor {
			out = append(out, p.URI())
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

type coverKey struct {
	postID uuid.UUID
	video  fedi.URL
}

// CoverRepository keeps the first frames of videos; a frame of a post the
// posts no longer keep is gone, as in PostgreSQL.
type CoverRepository struct {
	mu     sync.Mutex
	frames map[coverKey][]byte
	posts  *RemotePostRepository
}

func NewCoverRepository(posts *RemotePostRepository) *CoverRepository {
	return &CoverRepository{frames: map[coverKey][]byte{}, posts: posts}
}

func (r *CoverRepository) Get(ctx context.Context, postID uuid.UUID, video fedi.URL) ([]byte, error) {
	if _, err := r.posts.Get(ctx, postID); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	frame, ok := r.frames[coverKey{postID, video}]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return slices.Clone(frame), nil
}

func (r *CoverRepository) Save(_ context.Context, postID uuid.UUID, video fedi.URL, frame []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames[coverKey{postID, video}] = slices.Clone(frame)

	return nil
}

type likeKey struct {
	postID uuid.UUID
	userID uuid.UUID
}

// LikeRepository keeps the likes of remote posts; those of a post the
// posts no longer keep are gone, as in PostgreSQL.
type LikeRepository struct {
	mu    sync.Mutex
	likes map[likeKey]service.Liker
	posts *RemotePostRepository
}

func NewLikeRepository(posts *RemotePostRepository) *LikeRepository {
	return &LikeRepository{likes: map[likeKey]service.Liker{}, posts: posts}
}

func (r *LikeRepository) Add(ctx context.Context, postID uuid.UUID, l service.Liker, _ time.Time) error {
	if _, err := r.posts.Get(ctx, postID); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := likeKey{postID, l.UserID}
	if _, ok := r.likes[key]; !ok {
		r.likes[key] = l
	}

	return nil
}

func (r *LikeRepository) Remove(_ context.Context, postID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.likes, likeKey{postID, userID})

	return nil
}

func (r *LikeRepository) Of(ctx context.Context, postIDs []uuid.UUID, reader uuid.UUID) (map[uuid.UUID]service.Likes, error) {
	out := map[uuid.UUID]service.Likes{}

	for _, id := range postIDs {
		likers, err := r.Likers(ctx, id, uuid.Nil, 0)
		if err != nil {
			return nil, err
		}
		if len(likers) == 0 {
			continue
		}

		likes := service.Likes{Count: len(likers)}
		for _, l := range likers {
			likes.Liked = likes.Liked || l.UserID == reader
		}
		out[id] = likes
	}

	return out, nil
}

// Likers with limit 0 are all of them.
func (r *LikeRepository) Likers(ctx context.Context, postID, before uuid.UUID, limit int) ([]service.Liker, error) {
	// The likes of a post gone are gone.
	if _, err := r.posts.Get(ctx, postID); errors.Is(err, fedi.ErrNotFound) {
		return nil, nil
	}

	r.mu.Lock()
	var likers []service.Liker
	for key, l := range r.likes {
		if key.postID == postID && (before == uuid.Nil || l.ID.String() < before.String()) {
			likers = append(likers, l)
		}
	}
	r.mu.Unlock()

	slices.SortFunc(likers, func(a, b service.Liker) int { return cmp.Compare(b.ID.String(), a.ID.String()) })
	if limit > 0 && len(likers) > limit {
		likers = likers[:limit]
	}

	return likers, nil
}

// CommentRepository keeps the comments of remote posts; those of a post
// the posts no longer keep are gone, as in PostgreSQL.
type CommentRepository struct {
	mu       sync.Mutex
	comments map[uuid.UUID]*fedi.RemoteComment
	posts    *RemotePostRepository
}

func NewCommentRepository(posts *RemotePostRepository) *CommentRepository {
	return &CommentRepository{comments: map[uuid.UUID]*fedi.RemoteComment{}, posts: posts}
}

func (r *CommentRepository) AddComment(ctx context.Context, c *fedi.RemoteComment) error {
	if _, err := r.posts.Get(ctx, c.PostID()); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.comments[c.ID()] = c

	return nil
}

func (r *CommentRepository) Comment(ctx context.Context, id uuid.UUID) (*fedi.RemoteComment, error) {
	r.mu.Lock()
	c, ok := r.comments[id]
	r.mu.Unlock()

	if !ok || r.gone(ctx, c.PostID()) {
		return nil, fedi.ErrNotFound
	}

	return c, nil
}

func (r *CommentRepository) DeleteComment(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.comments, id)

	return nil
}

// Comments with limit 0 are all of them.
func (r *CommentRepository) Comments(ctx context.Context, postID, after uuid.UUID, limit int) ([]*fedi.RemoteComment, error) {
	if r.gone(ctx, postID) {
		return nil, nil
	}

	r.mu.Lock()
	var out []*fedi.RemoteComment
	for _, c := range r.comments {
		if c.PostID() == postID && c.ID().String() > after.String() {
			out = append(out, c)
		}
	}
	r.mu.Unlock()

	slices.SortFunc(out, func(a, b *fedi.RemoteComment) int { return cmp.Compare(a.ID().String(), b.ID().String()) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (r *CommentRepository) CommentCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}

	for _, id := range postIDs {
		comments, err := r.Comments(ctx, id, uuid.Nil, 0)
		if err != nil {
			return nil, err
		}
		if len(comments) > 0 {
			out[id] = len(comments)
		}
	}

	return out, nil
}

// gone: the comments of a post gone are gone.
func (r *CommentRepository) gone(ctx context.Context, postID uuid.UUID) bool {
	_, err := r.posts.Get(ctx, postID)
	return errors.Is(err, fedi.ErrNotFound)
}
