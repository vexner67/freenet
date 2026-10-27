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

// ArticleRepository keeps the articles of people. Articles handed out
// never change, so sharing pointers with callers is safe.
type ArticleRepository struct {
	mu       sync.RWMutex
	articles map[uuid.UUID]*post.Article
}

func NewArticleRepository() *ArticleRepository {
	return &ArticleRepository{
		articles: make(map[uuid.UUID]*post.Article),
	}
}

func (r *ArticleRepository) Save(ctx context.Context, a *post.Article) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.articles[a.ID()] = a

	return nil
}

func (r *ArticleRepository) Get(ctx context.Context, id uuid.UUID) (*post.Article, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.articles[id]
	if !ok {
		return nil, post.ErrArticleNotFound
	}

	return a, nil
}

func (r *ArticleRepository) Update(ctx context.Context, a *post.Article) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.articles[a.ID()]; !ok {
		return post.ErrArticleNotFound
	}

	r.articles[a.ID()] = a

	return nil
}

func (r *ArticleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.articles[id]; !ok {
		return post.ErrArticleNotFound
	}

	delete(r.articles, id)

	return nil
}

func (r *ArticleRepository) List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Article, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Article
	for _, a := range r.writtenBy(authorID) {
		id := a.ID()
		if before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0 {
			found = append(found, a)
		}
		if len(found) == limit {
			break
		}
	}

	return found, nil
}

func (r *ArticleRepository) Count(ctx context.Context, authorID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.writtenBy(authorID)), nil
}

// DeleteByUser deletes the articles of a user.
func (r *ArticleRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, a := range r.articles {
		if a.AuthorID() == userID {
			delete(r.articles, id)
		}
	}

	return nil
}

// writtenBy returns the articles of authorID, newest first; the caller
// holds the lock.
func (r *ArticleRepository) writtenBy(authorID uuid.UUID) []*post.Article {
	var written []*post.Article
	for _, a := range r.articles {
		if a.AuthorID() == authorID {
			written = append(written, a)
		}
	}

	slices.SortFunc(written, func(a, b *post.Article) int {
		ia, ib := a.ID(), b.ID()
		return bytes.Compare(ib[:], ia[:])
	})

	return written
}
