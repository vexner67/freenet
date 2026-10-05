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

type PostRepository struct {
	mu    sync.RWMutex
	posts map[uuid.UUID]*post.Post
}

func NewPostRepository() *PostRepository {
	return &PostRepository{
		posts: make(map[uuid.UUID]*post.Post),
	}
}

// Posts are immutable, so sharing pointers with callers is safe.
func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.posts[p.ID()] = p

	return nil
}

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.posts[id]
	if !ok {
		return nil, post.ErrNotFound
	}

	return p, nil
}

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.posts[id]; !ok {
		return post.ErrNotFound
	}

	delete(r.posts, id)

	return nil
}

func (r *PostRepository) ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error) {
	return r.ListByAuthors(ctx, []uuid.UUID{authorID}, before, limit)
}

func (r *PostRepository) ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Post
	for _, p := range r.posts {
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

	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, p := range r.posts {
		if p.AuthorID() == authorID {
			count++
		}
	}

	return count, nil
}
