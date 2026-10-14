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
)

// PostponedRepository keeps postponed posts apart from posts, as the
// table does in PostgreSQL. An edit replaces a postponed post rather than
// changes it, so sharing pointers with callers is safe.
type PostponedRepository struct {
	mu        sync.RWMutex
	postponed map[uuid.UUID]*post.Postponed
}

func NewPostponedRepository() *PostponedRepository {
	return &PostponedRepository{
		postponed: make(map[uuid.UUID]*post.Postponed),
	}
}

func (r *PostponedRepository) Save(ctx context.Context, p *post.Postponed) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.matching(func(q *post.Postponed) bool { return q.WallID() == p.WallID() })) >= post.MaxPostponed {
		return post.ErrTooManyPostponed
	}

	r.postponed[p.ID()] = p

	return nil
}

func (r *PostponedRepository) Get(ctx context.Context, id uuid.UUID) (*post.Postponed, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.postponed[id]
	if !ok {
		return nil, post.ErrPostponedNotFound
	}

	return p, nil
}

func (r *PostponedRepository) Update(ctx context.Context, p *post.Postponed) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.postponed[p.ID()]; !ok {
		return post.ErrPostponedNotFound
	}

	r.postponed[p.ID()] = p

	return nil
}

func (r *PostponedRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.postponed[id]; !ok {
		return post.ErrPostponedNotFound
	}

	delete(r.postponed, id)

	return nil
}

func (r *PostponedRepository) List(ctx context.Context, wallID uuid.UUID) ([]*post.Postponed, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.matching(func(p *post.Postponed) bool { return p.WallID() == wallID }), nil
}

func (r *PostponedRepository) Due(ctx context.Context, now time.Time, limit int) ([]*post.Postponed, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	due := r.matching(func(p *post.Postponed) bool { return !p.PublishAt().After(now) })

	return due[:min(limit, len(due))], nil
}

func (r *PostponedRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, p := range r.postponed {
		if p.AuthorID() == userID {
			delete(r.postponed, id)
		}
	}

	return nil
}

// matching returns the postponed posts that pick picks, the soonest
// first; the caller holds a lock.
func (r *PostponedRepository) matching(pick func(*post.Postponed) bool) []*post.Postponed {
	var found []*post.Postponed
	for _, p := range r.postponed {
		if pick(p) {
			found = append(found, p)
		}
	}

	slices.SortFunc(found, func(a, b *post.Postponed) int {
		if c := a.PublishAt().Compare(b.PublishAt()); c != 0 {
			return c
		}

		ai, bi := a.ID(), b.ID()

		return bytes.Compare(ai[:], bi[:])
	})

	return found
}
