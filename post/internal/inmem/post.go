package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// Store keeps posts, likes and comments together, so that likes and
// comments are counted in their post and go away with it, as the foreign
// keys do in PostgreSQL. Its repositories are Posts, Likes and Comments.
type Store struct {
	mu       sync.RWMutex
	posts    map[uuid.UUID]*post.Post
	likes    map[uuid.UUID]map[uuid.UUID]struct{} // post → users
	comments map[uuid.UUID]*post.Comment
}

func NewStore() *Store {
	return &Store{
		posts:    make(map[uuid.UUID]*post.Post),
		likes:    make(map[uuid.UUID]map[uuid.UUID]struct{}),
		comments: make(map[uuid.UUID]*post.Comment),
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

// count replaces a post with a copy whose counters are changed by update;
// posts are immutable, so callers may keep the pointers they got.
func (s *Store) count(id uuid.UUID, update func(*post.Counters)) {
	p := s.posts[id]
	counters := p.Counters()
	update(&counters)
	s.posts[id] = post.Restore(p.ID(), p.AuthorID(), p.Text(), p.CreatedAt(), counters)
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

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.posts[id]; !ok {
		return post.ErrNotFound
	}

	delete(r.s.posts, id)
	delete(r.s.likes, id)

	for commentID, c := range r.s.comments {
		if c.PostID() == id {
			delete(r.s.comments, commentID)
		}
	}

	return nil
}

func (r *PostRepository) ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.ListByAuthors(ctx, []uuid.UUID{authorID}, before, limit)
}

func (r *PostRepository) ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*post.Post
	for _, p := range r.s.posts {
		id := p.ID()
		if slices.Contains(authorIDs, p.AuthorID()) && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
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

func (r *PostRepository) CountByAuthor(ctx context.Context, authorID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	count := 0
	for _, p := range r.s.posts {
		if p.AuthorID() == authorID {
			count++
		}
	}

	return count, nil
}
