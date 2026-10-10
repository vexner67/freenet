package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// Store keeps posts, likes and comments together, so that likes and
// comments are counted in their post and go away with it, as the foreign
// keys do in PostgreSQL. Its repositories are Posts, Likes and Comments.
type Store struct {
	mu       sync.RWMutex
	posts    map[uuid.UUID]*post.Post
	likes    map[uuid.UUID]map[uuid.UUID]time.Time // post → users → when
	comments map[uuid.UUID]*post.Comment
	// commentLikes are comment → users → when, counted in the comments.
	commentLikes map[uuid.UUID]map[uuid.UUID]time.Time
	pins         map[uuid.UUID]uuid.UUID // wall → its pinned post
}

func NewStore() *Store {
	return &Store{
		posts:        make(map[uuid.UUID]*post.Post),
		likes:        make(map[uuid.UUID]map[uuid.UUID]time.Time),
		comments:     make(map[uuid.UUID]*post.Comment),
		commentLikes: make(map[uuid.UUID]map[uuid.UUID]time.Time),
		pins:         make(map[uuid.UUID]uuid.UUID),
	}
}

func (s *Store) Posts() *PostRepository {
	return &PostRepository{s: s}
}

func (s *Store) Likes() *LikeRepository {
	return &LikeRepository{s: s}
}

func (s *Store) Comments() *CommentRepository {
	return &CommentRepository{s: s}
}

func (s *Store) CommentLikes() *CommentLikeRepository {
	return &CommentLikeRepository{s: s}
}

// removeComment deletes a comment with its likes; its replies answer
// nothing then, as ON DELETE SET NULL does. Comments are immutable, so a
// reply is replaced by a copy.
func (s *Store) removeComment(id uuid.UUID) {
	delete(s.comments, id)
	delete(s.commentLikes, id)

	for replyID, c := range s.comments {
		if c.ReplyTo() == id {
			s.restoreComment(replyID, func(sc *post.StoredComment) { sc.ReplyTo = uuid.Nil })
		}
	}
}

// restoreComment replaces a comment with a copy changed by update.
func (s *Store) restoreComment(id uuid.UUID, update func(*post.StoredComment)) {
	c := s.comments[id]
	sc := post.StoredComment{
		ID: c.ID(), PostID: c.PostID(), AuthorID: c.AuthorID(), ReplyTo: c.ReplyTo(),
		Content: post.RestoreCommentContent(c.Text(), c.Photos()), CreatedAt: c.CreatedAt(), Likes: c.Likes(),
		FromWall: c.FromWall(),
	}
	update(&sc)
	s.comments[id] = post.RestoreComment(sc)
}

// count replaces a post with a copy whose counters are changed by update;
// posts are immutable, so callers may keep the pointers they got.
func (s *Store) count(id uuid.UUID, update func(*post.Counters)) {
	stored := s.posts[id].Stored()
	update(&stored.Counters)
	s.posts[id] = post.Restore(stored)
}

// setPinned replaces a post with a copy pinned or not, as count does.
func (s *Store) setPinned(id uuid.UUID, pinned bool) {
	stored := s.posts[id].Stored()
	stored.Pinned = pinned
	s.posts[id] = post.Restore(stored)
}

// remove deletes a post with its likes and pin; its comments the caller
// deletes.
func (s *Store) remove(p *post.Post) {
	delete(s.posts, p.ID())
	delete(s.likes, p.ID())

	if s.pins[p.WallID()] == p.ID() {
		delete(s.pins, p.WallID())
	}
}

type PostRepository struct {
	s *Store
}

func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if shared := p.Content().RepostOf(); shared != uuid.Nil {
		if _, ok := r.s.posts[shared]; !ok {
			return post.ErrNotFound
		}
		r.s.count(shared, func(c *post.Counters) { c.Reposts++ })
	}

	r.s.posts[p.ID()] = p

	return nil
}

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	p, ok := r.s.posts[id]
	if !ok {
		return nil, post.ErrNotFound
	}

	return p, nil
}

func (r *PostRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*post.Post
	for _, id := range ids {
		if p, ok := r.s.posts[id]; ok {
			found = append(found, p)
		}
	}

	return found, nil
}

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[id]
	if !ok {
		return post.ErrNotFound
	}

	// The shared post may be gone already: reposts outlive it.
	if shared := p.Content().RepostOf(); shared != uuid.Nil && r.s.posts[shared] != nil {
		r.s.count(shared, func(c *post.Counters) { c.Reposts-- })
	}

	r.s.remove(p)

	for commentID, c := range r.s.comments {
		if c.PostID() == id {
			r.s.removeComment(commentID)
		}
	}

	return nil
}

// ListByWall leaves out of all posts, unless searched, the pinned one,
// which the wall shows first.
func (r *PostRepository) ListByWall(ctx context.Context, wallID uuid.UUID, pick service.WallPick, before uuid.UUID, limit int) ([]*post.Post, error) {
	plain := pick.Filter == post.WallAll && pick.Query == ""

	return r.list(ctx, func(p *post.Post) bool {
		return p.WallID() == wallID && pick.Filter.Matches(p) && pick.Query.Matches(p) && (!plain || !p.Pinned())
	}, before, limit)
}

// SetArchived unpins the post it puts away, as PostgreSQL does.
func (r *PostRepository) SetArchived(ctx context.Context, id uuid.UUID, archived bool) error {
	return r.switchPost(ctx, id, func(s *post.Stored) {
		s.Archived = archived

		if archived && r.s.pins[s.WallID] == id {
			delete(r.s.pins, s.WallID)
			s.Pinned = false
		}
	})
}

func (r *PostRepository) SetCommentsClosed(ctx context.Context, id uuid.UUID, closed bool) error {
	return r.switchPost(ctx, id, func(s *post.Stored) { s.CommentsClosed = closed })
}

// switchPost replaces a post with a changed copy: posts handed out never
// change.
func (r *PostRepository) switchPost(ctx context.Context, id uuid.UUID, change func(*post.Stored)) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[id]
	if !ok {
		return post.ErrNotFound
	}

	stored := p.Stored()
	change(&stored)
	r.s.posts[id] = post.Restore(stored)

	return nil
}

func (r *PostRepository) Update(ctx context.Context, p *post.Post) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	old, ok := r.s.posts[p.ID()]
	if !ok {
		return post.ErrNotFound
	}

	// Only what an edit changes: the counters and pin are the store's.
	stored := old.Stored()
	stored.Content, stored.EditedAt = p.Content(), p.EditedAt()
	r.s.posts[p.ID()] = post.Restore(stored)

	return nil
}

func (r *PostRepository) Pin(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[id]
	if !ok {
		return post.ErrNotFound
	}

	if before, ok := r.s.pins[p.WallID()]; ok && before != id {
		r.s.setPinned(before, false)
	}

	r.s.pins[p.WallID()] = id
	r.s.setPinned(id, true)

	return nil
}

func (r *PostRepository) Unpin(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[id]
	if !ok || r.s.pins[p.WallID()] != id {
		return nil
	}

	delete(r.s.pins, p.WallID())
	r.s.setPinned(id, false)

	return nil
}

func (r *PostRepository) Pinned(ctx context.Context, wallID uuid.UUID) (*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	id, ok := r.s.pins[wallID]
	if !ok {
		return nil, post.ErrNotFound
	}

	return r.s.posts[id], nil
}

// ListFeed: a post that speaks for a community comes with its wall, not
// with its author, as in PostgreSQL.
func (r *PostRepository) ListFeed(ctx context.Context, sources service.FeedSources, before uuid.UUID, limit int) ([]*post.Post, error) {
	match := func(p *post.Post) bool {
		if p.Archived() {
			return false
		}

		return (slices.Contains(sources.Authors, p.AuthorID()) && !p.FromWall()) || slices.Contains(sources.Walls, p.WallID())
	}

	return r.list(ctx, match, before, limit)
}

// list returns up to limit matching posts, newest first, with IDs below
// before (uuid.Nil: from the newest).
func (r *PostRepository) list(ctx context.Context, match func(*post.Post) bool, before uuid.UUID, limit int) ([]*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*post.Post
	for _, p := range r.s.posts {
		id := p.ID()
		if match(p) && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			found = append(found, p)
		}
	}

	// Newest first: UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *post.Post) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(bi[:], ai[:])
	})

	if len(found) > limit {
		found = found[:limit]
	}

	return found, nil
}

func (r *PostRepository) Count(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	count := 0
	for _, p := range r.s.posts {
		if !p.Archived() {
			count++
		}
	}

	return count, nil
}

func (r *PostRepository) CountByWall(ctx context.Context, wallID uuid.UUID, pick service.WallPick) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	count := 0
	for _, p := range r.s.posts {
		if p.WallID() == wallID && pick.Filter.Matches(p) && pick.Query.Matches(p) {
			count++
		}
	}

	return count, nil
}

func (r *PostRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var deleted []*post.Post
	for _, p := range r.s.posts {
		if p.AuthorID() == userID || p.WallID() == userID {
			deleted = append(deleted, p)
		}
	}

	for _, p := range deleted {
		if shared := p.Content().RepostOf(); shared != uuid.Nil && r.s.posts[shared] != nil {
			r.s.count(shared, func(c *post.Counters) { c.Reposts-- })
		}
	}

	for _, p := range deleted {
		r.s.remove(p)
	}

	for commentID, c := range r.s.comments {
		if r.s.posts[c.PostID()] == nil {
			r.s.removeComment(commentID)
		}
	}

	return deleted, nil
}
